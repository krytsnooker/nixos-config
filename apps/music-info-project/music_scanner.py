# music_scanner.py
#
# Two-phase scan:
#   1. Fast os.walk to collect all paths and parse metadata from filenames
#      (no file opens — completes in seconds regardless of library size).
#   2. Parallel duration reads using a thread pool. Each worker opens only
#      the first ~270 bytes of a file: the ID3v2 header to locate audio
#      data, then one MP3 frame header (+ optional Xing VBR header) to get
#      an exact or estimated duration. A daemon sub-thread per file enforces
#      DURATION_TIMEOUT so a hung CIFS read never blocks the pool worker.
#
# Full wipe + rebuild on every run: `tracks` has no unique constraint, so
# re-running without clearing first would duplicate every row.

import os
import re
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed

from config import MUSIC_PATH
from database import get_db

AUDIO_EXTENSIONS  = {'.mp3', '.flac', '.m4a', '.ogg', '.opus', '.wav', '.aiff'}
DURATION_EXTS     = {'.mp3', '.flac'}
PROGRESS_EVERY    = 1000
DURATION_TIMEOUT  = 10   # seconds per file before giving up
WORKERS           = 16   # parallel CIFS readers

_TRACK_RE = re.compile(r'^(\d+)[.\s-]+\s*(.+)$')
_DISC_RE  = re.compile(r'^(disc|disk|cd)\s*\d+$', re.IGNORECASE)

_MP3_BITRATES    = [0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320]
_MP3_SAMPLERATES = [44100, 48000, 32000]


# ---------------------------------------------------------------------------
# Duration reading
# ---------------------------------------------------------------------------

def _duration_worker(path, out):
    """Reads the first ~270 bytes of a file to extract duration in seconds."""
    try:
        with open(path, 'rb') as f:
            header = f.read(10)

            # FLAC: STREAMINFO block is always first; 26 bytes total
            if header[:4] == b'fLaC':
                data = header + f.read(16)
                if len(data) >= 26:
                    n  = int.from_bytes(data[18:26], 'big')
                    sr = n >> 44
                    ts = n & 0xfffffffff
                    if sr > 0:
                        out[0] = int(round(ts / sr))
                return

            # MP3: skip ID3v2 tag to reach the first audio frame
            audio_start = 0
            if header[:3] == b'ID3' and len(header) == 10:
                sz = ((header[6] & 0x7f) << 21 | (header[7] & 0x7f) << 14 |
                      (header[8] & 0x7f) << 7  | (header[9] & 0x7f))
                if header[5] & 0x10:
                    sz += 10
                audio_start = 10 + sz

            f.seek(audio_start)
            frame     = f.read(256)
            file_size = os.fstat(f.fileno()).st_size

        if len(frame) < 4:
            return

        # Find frame sync word
        pos = 0
        while pos < len(frame) - 4:
            if frame[pos] == 0xFF and (frame[pos + 1] & 0xE0) == 0xE0:
                break
            pos += 1
        else:
            return

        b1, b2, b3 = frame[pos+1], frame[pos+2], frame[pos+3]
        if (b1 >> 3) & 0x3 != 3 or (b1 >> 1) & 0x3 != 1:  # MPEG1 Layer III only
            return

        br_idx = (b2 >> 4) & 0xF
        sr_idx = (b2 >> 2) & 0x3
        if br_idx == 0 or br_idx >= len(_MP3_BITRATES) or sr_idx >= len(_MP3_SAMPLERATES):
            return

        bitrate_kbps = _MP3_BITRATES[br_idx]
        sample_rate  = _MP3_SAMPLERATES[sr_idx]

        # Xing/Info VBR header gives exact frame count → exact duration
        xing_off = pos + (36 if (b3 >> 6) & 0x3 != 3 else 21)
        if xing_off + 12 <= len(frame) and frame[xing_off:xing_off+4] in (b'Xing', b'Info'):
            flags = int.from_bytes(frame[xing_off+4:xing_off+8], 'big')
            if flags & 0x1:
                total_frames = int.from_bytes(frame[xing_off+8:xing_off+12], 'big')
                out[0] = int(round(total_frames * 1152 / sample_rate))
                return

        # CBR fallback: file size / bitrate
        out[0] = int(round((file_size - audio_start) * 8 / (bitrate_kbps * 1000)))

    except Exception:
        pass


def _read_duration(path):
    """Reads duration in a daemon thread; returns 0 on timeout or error."""
    out = [0]
    t = threading.Thread(target=_duration_worker, args=(path, out), daemon=True)
    t.start()
    t.join(DURATION_TIMEOUT)
    return out[0]


# ---------------------------------------------------------------------------
# Path parsing
# ---------------------------------------------------------------------------

def _parse_filename(fname):
    stem = os.path.splitext(fname)[0]
    m = _TRACK_RE.match(stem)
    if m:
        return int(m.group(1)), m.group(2).strip()
    return 0, stem.strip()


def _parse_path(rel_path):
    parts    = rel_path.replace('\\', '/').split('/')
    track_no, title = _parse_filename(parts[-1])

    if len(parts) == 3:
        artist, album = parts[0], parts[1]
    elif len(parts) == 4 and _DISC_RE.match(parts[2]):
        artist, album = parts[0], parts[1]
    elif len(parts) >= 3:
        artist, album = parts[-3], parts[-2]
    else:
        artist, album = 'Unknown Artist', 'Unknown Album'

    return artist, album, track_no, title


# ---------------------------------------------------------------------------
# Main scan
# ---------------------------------------------------------------------------

def scan():
    if not os.path.isdir(MUSIC_PATH):
        print(f"ERROR: MUSIC_PATH does not exist or isn't mounted: {MUSIC_PATH}", file=sys.stderr)
        sys.exit(1)

    total_start = time.time()

    # Phase 1: directory walk — no file opens
    print(f"Scanning {MUSIC_PATH} ...")
    entries = []   # list of (rel_path, full_path, ext)
    for root, _dirs, files in os.walk(MUSIC_PATH):
        for fname in files:
            ext = os.path.splitext(fname)[1].lower()
            if ext not in AUDIO_EXTENSIONS:
                continue
            full_path = os.path.join(root, fname)
            rel       = os.path.relpath(full_path, MUSIC_PATH)
            entries.append((rel, full_path, ext))

    walk_time = time.time() - total_start
    print(f"Found {len(entries)} audio files in {walk_time:.1f}s. Reading durations with {WORKERS} workers...")

    # Phase 2: parallel duration reads
    dur_start  = time.time()
    durations  = [0] * len(entries)
    audio_jobs = [(i, e[1]) for i, e in enumerate(entries) if e[2] in DURATION_EXTS]

    completed = 0
    with ThreadPoolExecutor(max_workers=WORKERS) as pool:
        futures = {pool.submit(_read_duration, path): i for i, path in audio_jobs}
        for future in as_completed(futures):
            durations[futures[future]] = future.result()
            completed += 1
            if completed % PROGRESS_EVERY == 0:
                print(f"  ...{completed}/{len(audio_jobs)} durations read")

    dur_time = time.time() - dur_start
    print(f"Durations read in {dur_time:.1f}s. Writing to database...")

    # Phase 3: build rows and write DB
    rows    = []
    skipped = 0
    for i, (rel, full_path, ext) in enumerate(entries):
        try:
            artist, album, track_no, title = _parse_path(rel)
            rows.append((artist, album, title, track_no, durations[i]))
        except Exception as e:
            print(f"WARNING: could not parse '{rel}': {e}", file=sys.stderr)
            skipped += 1

    with get_db() as conn:
        conn.execute('''CREATE TABLE IF NOT EXISTS tracks
                      (artist TEXT, album TEXT, title TEXT, track_no INTEGER, duration INTEGER)''')
        conn.execute('DELETE FROM tracks')
        conn.executemany(
            'INSERT INTO tracks (artist, album, title, track_no, duration) VALUES (?, ?, ?, ?, ?)',
            rows,
        )
        conn.commit()

    elapsed = time.time() - total_start
    print(f"Done. {len(rows)} tracks written ({skipped} skipped) in {elapsed:.1f}s total.")


if __name__ == '__main__':
    scan()
