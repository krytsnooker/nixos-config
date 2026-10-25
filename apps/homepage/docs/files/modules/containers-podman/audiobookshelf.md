# modules/containers-podman/audiobookshelf.nix

Defines the Audiobookshelf Podman container. Audiobookshelf serves audio books and podcasts from the DAS media share.

## Image

Pinned by digest: `ghcr.io/advplyr/audiobookshelf@sha256:e388e90e…`. Update deliberately.

## Container configuration

| Setting | Value |
|---|---|
| Port | `config.homeserver.ports.audiobookshelf:80/tcp` (13378 → 80) |
| Config volume | `/var/lib/audiobookshelf/config:/config` |
| Metadata volume | `/var/lib/audiobookshelf/metadata:/metadata` |
| Media volume | `config.homeserver.paths.media:/media` |
| `TZ` | `config.time.timeZone` |
| `PUID` | `config.homeserver.uids.podman.audiobookshelf` (979) |
| `PGID` | `config.homeserver.gids.sambashare` (125) |

The library path inside the container is `/media/Audio Books`.

## User extension

`users.users.audiobookshelf.extraGroups = [ "sambashare" ]` — required to read the DAS media files. Base user created by `podman.nix`.

## State directories

Two directories created via `systemd.tmpfiles.rules`: `config` and `metadata`. Both use `config.homeserver.podman.tmpfiles` defaults.

## Firewall

Opens port `audiobookshelf` (13378).
