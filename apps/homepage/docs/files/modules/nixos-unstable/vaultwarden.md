# modules/nixos-unstable/vaultwarden.nix

Configures Vaultwarden (a Bitwarden-compatible server) using the unstable overlay package. Listens on localhost only; exposed via nginx reverse proxy.

## Package source

`vaultwarden` and `vaultwarden-webvault` are both pulled from `nixpkgs-unstable` via the overlay in `flake.nix`.

## Configuration

| Setting | Value |
|---|---|
| `dbBackend` | `sqlite` |
| `backupDir` | `/var/backup/vaultwarden` |
| `environmentFile` | `/var/lib/vaultwarden/secrets.env` |
| `DOMAIN` | `https://vault.intrentaka.com` |
| `SIGNUPS_ALLOWED` | `false` |
| `ROCKET_ADDRESS` | `config.homeserver.localhost` (127.0.0.1) |
| `ROCKET_PORT` | 8222 (module-local constant, not in config-values.nix) |
| `SHOW_PASSWORD_HINT` | `false` |

## Nginx proxy

The `vault.intrentaka.com` vhost proxies to `http://127.0.0.1:8222` with `proxyWebsockets = true` (required for real-time vault sync). TLS via Let's Encrypt ACME.

## Backup

`backupDir = "/var/backup/vaultwarden"` — the NixOS vaultwarden module creates a daily SQLite backup job that writes to this path. Backup files are also manually copied to Vaultwarden itself as secure notes under `homeserver/`.

## Secrets

`/var/lib/vaultwarden/secrets.env` holds `ADMIN_TOKEN` and any SMTP credentials. See `check-secrets.sh` for expected keys.

## Notes

- `ROCKET_PORT` is a module-local `let` binding (8222) rather than a `config-values.nix` entry because Vaultwarden never needs to bind on the external interface — only nginx needs to know where to proxy to, and that reference is in the same file.
- No sandbox hardening applied — the NixOS vaultwarden module already hardens the service.
