# config-values.nix

Declares all shared numeric and string constants as typed NixOS options under the `homeserver.*` namespace. Every module reads from this file; nothing is written back to it at runtime. It is the single source of truth for ports, UIDs, GIDs, filesystem paths, sandbox flags, and the loopback address.

## Why a NixOS options file rather than a plain let-binding

Using `lib.mkOption` makes each value type-checked and overridable per-host. Modules reference values as `config.homeserver.<group>.<name>` — the same way any NixOS option is consumed — so adding a second host in the future only requires a new `config-values.nix` with different defaults.

## Option groups

### uids

UIDs for all non-login system accounts. Divided into three sub-groups matching how the accounts are managed:

| Sub-group | Used by |
|---|---|
| `uids.podman` | `modules/containers-podman/podman.nix` — creates users/groups in a single `mapAttrs` loop |
| `uids.unstable` | `modules/nixos-unstable/nextcloud.nix` and the CIFS mount in `configuration.nix` — must stay in sync |
| `uids.services` | `modules/services/lan-apps.nix` (music-info, rom-transfer) and `modules/services/minecraft.nix` (mcctl) |

NixOS native services (Loki, Grafana, murmur, etc.) manage their own UIDs internally — they are not pinned here.

### gids

GIDs for groups shared across more than one user or service. All declared at the same level (not sub-grouped):

| Name | Value | Shared by |
|---|---|---|
| `sambashare` | 125 | CIFS mount options (`gid=`), Emby (`PGID=`), music-info, rom-transfer, nextcloud, kryt |
| `musicInfo` | 991 | `music-info` group; also used as the Samba-share group for music-info service |
| `romTransfer` | 979 | `rom-transfer` group |
| `mcctl` | 993 | `mcctl` group (mc-control service) |
| `mcmods` | 980 | `mcmods` group (shared between mcctl and minecraft users for the mods directory) |

### paths

Absolute paths to directories on the DAS CIFS mount. Used by services that read or write media files:

| Name | Value | Referenced by |
|---|---|---|
| `paths.media` | `/mnt/server-pc/Media` | Emby, Audiobookshelf, music-info, music-scanner |
| `paths.backups` | `/mnt/server-pc/Backups` | `mc-backup` timer |

### sandbox

An attrset of systemd hardening flags applied to all custom services and NixOS native services via `// config.homeserver.sandbox` or a separate `serviceConfig` block. Changing a flag here propagates to every service that uses it on the next rebuild.

| Flag | Value |
|---|---|
| `NoNewPrivileges` | `true` |
| `PrivateTmp` | `true` |
| `ProtectKernelTunables` | `true` |
| `ProtectKernelModules` | `true` |
| `ProtectControlGroups` | `true` |
| `RestrictSUIDSGID` | `true` |
| `LockPersonality` | `true` |

Applied to all services. Exceptions:

| Service | Exception |
|---|---|
| | |

### localhost / addresses

| Name | Value | Referenced by |
|---|---|---|
| `localhost` | `127.0.0.1` | Vaultwarden `ROCKET_ADDRESS`, Grafana → Loki URL, Alloy push URL, Nextcloud `trusted_proxies` |
| `lanSubnet` | `192.168.0.0/24` | vpn-confinement `accessibleFrom` — allows LAN hosts to reach the `wg` namespace via the host veth |

### ports

All service listen ports. Referenced in module `ExecStart` arguments, nginx `proxyPass` targets, and `networking.firewall.allowedTCPPorts` lists.

| Name | Value | Service |
|---|---|---|
| `homepage` | 5000 | nginx static site |
| `lanPastebin` | 5001 | lan-pastebin Flask app |
| `typingTutor` | 3000 | typing-tutor Node app |
| `mathTutor` | 3001 | math-tutor Node app |
| `musicInfo` | 5010 | music-info Flask app |
| `romTransfer` | 5035 | rom-transfer Go server |
| `mcControl` | 5020 | mc-control Flask app |
| `emby` | 8096 | Emby container HTTP |
| `embyHttps` | 8920 | Emby container HTTPS |
| `audiobookshelf` | 13378 | Audiobookshelf container |
| `pihole` | 8083 | Pi-hole web UI (external) |
| `piholeInternal` | 8080 | Pi-hole web UI (container-internal, via `FTLCONF_webserver_port`) |
| `dns` | 53 | Pi-hole DNS |
| `minecraft` | 25565 | Minecraft server |
| `grafana` | 3010 | Grafana |
| `loki` | 3100 | Loki |
| `qbittorrent` | 8090 | qBittorrent web UI, VueTorrent alternative UI enabled (VPN namespace, exposed to LAN via `portMappings`) |
| `qbittorrentTorrent` | 6881 | qBittorrent torrent port (opened in VPN namespace via `openVPNPorts`) |
