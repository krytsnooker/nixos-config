# modules/containers-podman/emby.nix

Defines the Emby media server Podman container. Emby runs as a non-root user with hardware transcoding access.

## Image

Pinned by digest: `emby/embyserver@sha256:3aafff93…`. Never updated by floating tag — update the digest deliberately when upgrading.

## Container configuration

| Setting | Value |
|---|---|
| Ports | `8096:8096/tcp`, `8920:8920/tcp` (from `config.homeserver.ports`) |
| Config volume | `/var/lib/emby/config:/config` |
| Media volume | `config.homeserver.paths.media:/media` |
| `PUID` | `config.homeserver.uids.podman.emby` (980) |
| `PGID` | `config.homeserver.gids.sambashare` (125) |
| Device passthrough | `/dev/dri:/dev/dri` (Intel Quick Sync hardware transcoding) |

## UID/GID constraints

- `PUID` must match the `emby` system user UID declared in `config-values.nix`. If these diverge, Emby cannot write to `/var/lib/emby/config/`. Run `chown -R emby:emby /var/lib/emby/config/` after any UID change.
- `PGID` = `sambashare` (125) so Emby can read media files owned by the DAS CIFS mount.

## User extension

`users.users.emby.extraGroups = [ "video" "sambashare" ]` — added here rather than in `podman.nix` because `video` and `sambashare` membership is Emby-specific. The base `emby` user/group is created by `podman.nix`.

## State directory

Created by `systemd.tmpfiles.rules` with `mode` and `age` from `config.homeserver.podman.tmpfiles`. Only the config subdirectory is managed here; media is on the DAS.

## Firewall

Opens ports `emby` (8096) and `embyHttps` (8920).
