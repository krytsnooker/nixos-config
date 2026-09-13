# music_scanner.py
#
# Walks MUSIC_PATH and derives track metadata from the directory structure
# rather than opening files. Expects: MUSIC_PATH/Artist/Album/NN - Title.ext
# Disc subdirectories (Artist/Album/Disc N/NN - Title.ext) are also handled.
#
# Full wipe + rebuild on every run: `tracks` has no unique constraint, so
# re-running without clearing first would duplicate every row.

import os
import re
import sys
import time

from config import MUSIC_PATH
from database import get_db

AUDIO_EXTENSIONS = {'.mp3', '.flac', '.m4a', '.ogg', '.opus', '.wav', '.aiff'}
PROGRESS_EVERY = 500

# Matches leading track numbers like: "01 - ", "1. ", "02 "
_TRACK_RE = re.compile(r'^(\d+)[.\s-]+\s*(.+)$')
# Disc/CD subdirectory names to treat as part of the album, not a sub-artist
_DISC_RE = re.compile(r'^(disc|disk|cd)\s*\d+$', re.IGNORECASE)


def _parse_filename(fname):
    stem = os.path.splitext(fname)[0]
    m = _TRACK_RE.match(stem)
    if m:
        return int(m.group(1)), m.group(2).strip()
    return 0, stem.strip()


def _parse_path(rel_path):
    """
    Returns (artist, album, track_no, title) from a relative path.
    Handles both Artist/Album/file and Artist/Album/Disc N/file layouts.
    """
    parts = rel_path.replace('\\', '/').split('/')
    fname = parts[-1]
    track_no, title = _parse_filename(fname)

    if len(parts) == 3:
        artist, album = parts[0], parts[1]
    elif len(parts) == 4 and _DISC_RE.match(parts[2]):
        artist, album = parts[0], parts[1]
    elif len(parts) >= 3:
        # Deeper nesting — use the two levels above the file
        artist, album = parts[-3], parts[-2]
    else:
        artist, album = 'Unknown Artist', 'Unknown Album'

    return artist, album, track_no, title


def scan():
    if not os.path.isdir(MUSIC_PATH):
        print(f"ERROR: MUSIC_PATH does not exist or isn't mounted: {MUSIC_PATH}", file=sys.stderr)
        sys.exit(1)

    start = time.time()
    scanned = 0
    skipped = 0
    rows = []

    print(f"Scanning {MUSIC_PATH} ...")
    for root, _dirs, files in os.walk(MUSIC_PATH):
        for fname in files:
            ext = os.path.splitext(fname)[1].lower()
            if ext not in AUDIO_EXTENSIONS:
                continue

            scanned += 1
            rel = os.path.relpath(os.path.join(root, fname), MUSIC_PATH)

            try:
                artist, album, track_no, title = _parse_path(rel)
                rows.append((artist, album, title, track_no, 0))
            except Exception as e:
                print(f"WARNING: could not parse '{rel}': {e}", file=sys.stderr)
                skipped += 1

            if scanned % PROGRESS_EVERY == 0:
                print(f"  ...{scanned} files scanned so far")

    inserted = len(rows)
    print(f"Scan complete: {inserted} tracks read, {skipped} skipped, {scanned} files examined.")
    print("Rebuilding tracks table...")

    with get_db() as conn:
        conn.execute('''CREATE TABLE IF NOT EXISTS tracks
                      (artist TEXT, album TEXT, title TEXT, track_no INTEGER, duration INTEGER)''')
        conn.execute('DELETE FROM tracks')
        conn.executemany(
            'INSERT INTO tracks (artist, album, title, track_no, duration) VALUES (?, ?, ?, ?, ?)',
            rows,
        )
        conn.commit()

    elapsed = time.time() - start
    print(f"Done. {inserted} tracks written to the database in {elapsed:.1f}s.")


if __name__ == '__main__':
    scan()
