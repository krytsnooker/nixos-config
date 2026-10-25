# modules/nixos-stable/promtail.nix

Ships all journald entries to Loki and attaches structured labels to every log line. The relabeling logic is the most complex part of this module.

## Transport

- Scrapes journald directly (`journal` scrape type with `max_age = "12h"`)
- Pushes to `http://127.0.0.1:3100/loki/api/v1/push`
- HTTP listen port: `config.homeserver.ports.promtail` (9080) — for Grafana's Promtail data source health checks

## Labels

Every log entry receives five labels:

| Label | Source | Example |
|---|---|---|
| `host` | Static | `homeserver` |
| `unit` | `__journal__systemd_unit` verbatim | `nginx.service` |
| `level` | `__journal_priority_keyword` | `info`, `err`, `crit` |
| `category` | Relabel rules | `system`, `nixos-modules`, `nixos-sys` |
| `module` | Relabel rules | `kernel`, `nixos-stable`, `services` |
| `service` | Relabel rules | Unit name with `.service`/`.scope` stripped, or overridden |

## Relabeling strategy

Rules are applied in order; later rules override earlier ones. This means defaults are set first, then progressively overridden by more specific patterns.

**Passthrough** — copies `unit` and `level` labels verbatim.

**Defaults** — sets `service` to the unit name minus `.service`/`.scope` suffix; sets `category=system`, `module=unknown` for everything.

Then specific groups override `category` and `module`:

| category | module | Matched units |
|---|---|---|
| `system` | `kernel` | systemd-journald, systemd-logind, systemd-udevd, coredump, fsck, user sessions |
| `system` | `hardware` | ModemManager, bluetooth, fwupd, irqbalance, thermald, udisks2 |
| `system` | `networking` | NetworkManager, wpa_supplicant, avahi, samba-wsdd |
| `system` | `desktop` | accounts-daemon, cups, dbus, display-manager, polkit, rtkit, drkonqi |
| `nixos-sys` | `acme` | `acme-*.service` (Let's Encrypt) |
| `nixos-sys` | `nix` | nix-daemon, nixos-rebuild |
| `nixos-sys` | `system` | cron, logrotate, nscd |
| `nixos-sys` | `remote` | xrdp, xrdp-sesman |
| `nixos-modules` | `nixos-stable` | fail2ban, grafana, loki, promtail, murmur, mysql, postgresql |
| `nixos-modules` | `nixos-unstable` | vaultwarden, phpfpm-nextcloud, redis-nextcloud, nextcloud-* |
| `nixos-modules` | `containers-podman` | `podman-*.service` (service name extracted from unit name) |
| `nixos-modules` | `services` | math-tutor, typing-tutor, music-info, rom-transfer, mc-control, minecraft, nginx, lan-pastebin |

Anything not matched by a specific group stays at `category=system`, `module=unknown`.

## Hardening

`systemd.services.promtail.serviceConfig = config.homeserver.sandbox` applies the standard sandbox flags.
