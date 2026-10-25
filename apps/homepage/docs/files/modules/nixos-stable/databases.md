# modules/nixos-stable/databases.nix

Enables the two database engines used by NixOS-managed services. No custom configuration — both run with NixOS defaults.

## Services

| Service | Package | Used by |
|---|---|---|
| MariaDB | `pkgs.mariadb` | Nextcloud (`database.createLocally = true` in `nextcloud.nix` handles schema and user creation) |
| PostgreSQL 16 | `pkgs.postgresql_16` | Available for future use; not currently consumed by any module |

## Notes

- Nextcloud's `database.createLocally = true` means the `nextcloud.nix` module instructs the MariaDB service to create the `nextcloud` database and user automatically on first start.
- No sandbox hardening is applied — NixOS's own MariaDB and PostgreSQL modules already harden their services.
- No firewall ports opened; both databases listen on localhost only.
