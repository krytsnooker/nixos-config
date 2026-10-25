# modules/services/lan-apps.nix

Defines all custom LAN application services: the static homepage, four Flask/Node web apps, and two Go services (rom-transfer, music-info). All source code lives in `apps/` and is referenced by Nix path expressions.

## Homepage (nginx static)

Served by nginx from `../../apps/homepage`. Uses a custom `homepage-lan` virtualHost listening on `config.homeserver.ports.homepage` (5000). No TLS — LAN only. A rebuild is required to pick up changes to the static files.

## LAN Pastebin

- Python Flask + waitress
- `DynamicUser = true` — ephemeral UID assigned by systemd; no persistent user entry
- State at `/var/lib/lan-pastebin/pastes.db` (SQLite)
- Port: `config.homeserver.ports.lanPastebin` (5001)

## Math Tutor

- Node.js / Express
- `DynamicUser = true`
- `express` npm package installed once to `/var/lib/math-tutor/node_modules` via `ExecStartPre` — skipped if already present
- State (logs) at `/var/lib/math-tutor/logs.json`
- Port: `config.homeserver.ports.mathTutor` (3001)

## Typing Tutor

- Node.js / Express
- Same npm install-once pattern as math-tutor
- State (logs) at `/var/lib/typing-tutor/logs.json`
- Port: `config.homeserver.ports.typingTutor` (3000)

## Music Info

- Python Flask + waitress
- Pinned system user `music-info` (UID `config.homeserver.uids.services.musicInfo`, GID `config.homeserver.gids.musicInfo`)
- `extraGroups = [ "sambashare" ]` — needs DAS access to read media files
- State at `/var/lib/music-info/music.db` (SQLite)
- `EnvironmentFile` at `/var/lib/music-info/secrets.env`
- Depends on `mnt-server-pc.mount` (DAS) — `after` + implicitly waits for mount
- Port: `config.homeserver.ports.musicInfo` (5010)

## Music Scanner

- Oneshot service (manual trigger only — no timer)
- Runs as `music-info` user
- Reads `MUSIC_PATH` (DAS `Albums` directory), writes to the same SQLite database
- Triggered with `sudo systemctl start music-scanner`; follow progress with `journalctl -fu music-scanner`
- ~27,000 tracks; completes in ~13 minutes with 16 parallel CIFS workers

## ROM Transfer

- Go binary built inline via `pkgs.buildGoModule`
- Pinned system user `rom-transfer` (UID `config.homeserver.uids.services.romTransfer`, GID `config.homeserver.gids.romTransfer`)
- `extraGroups = [ "sambashare" ]` — reads/writes ROM files on the DAS
- `ExecStartPre` copies the `web/` directory from the Nix store to `/var/lib/rom-transfer/web/` (writable copy needed for uploads)
- Config file at `/var/lib/rom-transfer/hostconfig.json`
- `EnvironmentFile` at `/var/lib/rom-transfer/secrets.env` (optional — prefixed with `-`)
- Depends on `mnt-server-pc.mount`
- Port: `config.homeserver.ports.romTransfer` (5035)

## Shared Python environment

A single `pkgs.python3.withPackages` expression (`flask`, `waitress`, `requests`, `mutagen`) is used for both lan-pastebin and music-info. Adding a new Python app may require extending this list.

## Firewall

Opens: `typingTutor` (3000), `mathTutor` (3001), `homepage` (5000), `lanPastebin` (5001), `musicInfo` (5010), `romTransfer` (5035).

## Sandbox

All services use `// config.homeserver.sandbox` (custom) or `config.homeserver.sandbox` (NixOS-native-style). `DynamicUser` services get implicit sandbox hardening from systemd in addition.
