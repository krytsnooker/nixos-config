# music_scanner.py
#
# Walks MUSIC_PATH (see config.py) for audio files, reads their tags, and
# rebuilds the `tracks` table used by app.py. This is a RECONSTRUCTION —
# the original scanner was lost (overwritten with a copy of database.py on
# both the live server and the exported copy) — written from the schema
# already present in music.db and the behavior described in
# scan_and_run.sh/run_app.sh ("Full Refresh (Scan + Run)").
#
# Full wipe + rebuild on every run: `tracks` has no unique constraint, so
# re-running without clearing first would duplicate every row.

import os
import sys
import time

from mutagen import File as MutagenFile

from config import MUSIC_PATH
from database import get_db

AUDIO_EXTENSIONS = {'.mp3', '.flac', '.m4a'}
PROGRESS_EVERY = 500


def _first_tag(tags, key, default=""):
    if not tags:
        return default
    values = tags.get(key)
    if not values:
        return default
    return str(values[0])


def _parse_track_no(raw):
    """Handles 'tracknumber' tags like '7', '7/12', or missing entirely."""
    if not raw:
        return 0
    digits = raw.split('/')[0].strip()
    try:
        return int(digits)
    except ValueError:
        return 0


def _read_track(path):
    audio = MutagenFile(path, easy=True)
    if audio is None:
        return None

    tags = audio.tags
    artist = _first_tag(tags, 'artist', 'Unknown Artist')
    album = _first_tag(tags, 'album', 'Unknown Album')
    title = _first_tag(tags, 'title', os.path.splitext(os.path.basename(path))[0])
    track_no = _parse_track_no(_first_tag(tags, 'tracknumber', ''))

    duration = 0
    if audio.info is not None and hasattr(audio.info, 'length'):
        duration = int(round(audio.info.length))

    return (artist, album, title, track_no, duration)


def scan():
    if not os.path.isdir(MUSIC_PATH):
        print(f"ERROR: MUSIC_PATH does not exist or isn't mounted: {MUSIC_PATH}", file=sys.stderr)
        sys.exit(1)

    start = time.time()
    scanned = 0
    inserted = 0
    skipped = 0
    rows = []

    print(f"Scanning {MUSIC_PATH} ...")
    for root, _dirs, files in os.walk(MUSIC_PATH):
        for fname in files:
            ext = os.path.splitext(fname)[1].lower()
            if ext not in AUDIO_EXTENSIONS:
                continue

            scanned += 1
            path = os.path.join(root, fname)
            try:
                track = _read_track(path)
            except Exception as e:
                print(f"WARNING: could not read '{path}': {e}", file=sys.stderr)
                track = None

            if track is None:
                skipped += 1
                continue

            rows.append(track)
            inserted += 1

            if scanned % PROGRESS_EVERY == 0:
                print(f"  ...{scanned} files scanned, {inserted} tracks read so far")

    print(f"Scan complete: {inserted} tracks read, {skipped} files skipped, {scanned} files examined.")
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
