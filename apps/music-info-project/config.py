# config.py
import os
import sys

DB_PATH = os.environ.get("MUSIC_DB_PATH", "music.db")
PORT = int(os.environ.get("MUSIC_INFO_PORT", 5010))

# Music library root, as seen from this machine (the CIFS mount of the DAS).
MUSIC_PATH = os.environ.get("MUSIC_PATH", "/mnt/server-pc/Media/Music")

# Secrets are read from the environment, never hardcoded here — this file
# ends up readable by any local user once packaged (Nix store, etc).
# Set these via a systemd EnvironmentFile (outside the Nix store) or a local
# .env you source before running directly.
DISCOGS_TOKEN = os.environ.get("DISCOGS_TOKEN", "")
EMBY_API_KEY = os.environ.get("EMBY_API_KEY", "")

if not DISCOGS_TOKEN:
    print("WARNING: DISCOGS_TOKEN not set — Discogs lookups will be skipped.", file=sys.stderr)
if not EMBY_API_KEY:
    print("WARNING: EMBY_API_KEY not set — Emby deep-linking will be disabled.", file=sys.stderr)

# Wikipedia & MusicBrainz require a descriptive User-Agent
HEADERS = {
    'User-Agent': os.environ.get(
        "MUSIC_INFO_USER_AGENT",
        "HomeMusicLibraryManager/1.4 (contact: set MUSIC_INFO_USER_AGENT)",
    )
}

EMBY_BASE_URL = os.environ.get("EMBY_BASE_URL", "http://localhost:8096")
EMBY_EXTERNAL_URL = os.environ.get("EMBY_EXTERNAL_URL", "http://localhost:8096")
