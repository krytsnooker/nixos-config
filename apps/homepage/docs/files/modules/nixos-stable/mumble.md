# modules/nixos-stable/mumble.nix

Configures Murmur (the Mumble server daemon). Minimal configuration; relies on NixOS defaults for most settings.

## Configuration

| Setting | Value |
|---|---|
| `registerName` | `hometalk` |
| `bandwidth` | 72000 bps |
| `users` | 20 (maximum concurrent) |
| `openFirewall` | true (opens TCP + UDP on port 64738) |

Server data (user database, certificates) is stored at `/var/lib/murmur`.

## Notes

- No password configured — LAN-only use.
- `openFirewall = true` is used instead of a manual `networking.firewall` entry to keep this module self-contained.
- No sandbox hardening applied — the NixOS murmur module already restricts the service.
