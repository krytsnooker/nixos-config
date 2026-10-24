# Adding a new NixOS module

One service = one file. Never group multiple services into a single module file unless they are tightly coupled infrastructure (e.g. databases). This makes each service independently upgradeable and patchable.

Do not write a single line of a new module until you have completed Steps 1 and 2.

---

## Step 1 — Read these files first

- `modules/config-values.nix` — all declared ports, UIDs, GIDs, paths, sandbox options
- The closest existing module to what you are adding (see reference table below)
- `flake.nix` — to confirm the correct group and ordering for the new module

| Adding a... | Read this as reference |
|---|---|
| NixOS native service (current stable channel) | `modules/nixos-stable/databases.nix`, `modules/nixos-stable/mumble.nix` |
| NixOS native service (unstable overlay) | `modules/nixos-unstable/vaultwarden.nix` |
| Podman container | `modules/containers-podman/emby.nix`, `modules/containers-podman/pihole.nix` |
| Custom systemd service | `modules/services/lan-apps.nix` (music-info section is the most complete example) |

---

## Step 2 — Answer every question before writing

**Port**
- Does the service need a port? Add it to `ports` in `modules/config-values.nix`.
- If the service has an internal-only port (e.g. Loki, Promtail) it still goes in config-values — no hardcoding.

**Secrets**
- Does the service need secrets? The file is always `/var/lib/<service>/secrets.env`, mode 600, owned by `<service>:<service>`.
- NixOS native services: inject via `systemd.services.<name>.serviceConfig.EnvironmentFile`.
- Custom systemd services: inject via `EnvironmentFile` inside the serviceConfig block, merged with `// config.homeserver.sandbox`.
- After writing the module, add a check to `check-secrets.sh` and a row to the secrets table in `CLAUDE.md`.

**Package sourcing**
- Check CLAUDE.md for the current stable channel version before deciding where a module belongs.
- Is the package in the current stable channel? Put the module in `modules/nixos-stable/`. No explicit `package =` needed unless choosing a specific version (e.g. `pkgs.postgresql_16`).
- Is the package only available or needed from unstable? Add it to the overlay in `flake.nix` and put the module in `modules/nixos-unstable/`.

**System user and group**
- NixOS native services (`services.X`) manage their own user — do not declare one.
- Custom systemd services need explicit `users.groups.<name> = {};` and `users.users.<name> = { isSystemUser = true; group = "<name>"; };` before the service definition.
- If the service needs CIFS/Samba access, add it to `extraGroups = [ "sambashare" ]`.

**Hardening (required for every service — do not skip)**
- Custom systemd service: `serviceConfig = { ... } // config.homeserver.sandbox;`
- NixOS native service: separate block — `systemd.services.<name>.serviceConfig = config.homeserver.sandbox;`
- NixOS native service with EnvironmentFile: `systemd.services.<name>.serviceConfig = config.homeserver.sandbox // { EnvironmentFile = "..."; };`
- Exempt: databases, nginx — their NixOS modules already harden them.

**Firewall**
- Only open ports that LAN clients need to reach directly.
- Always use: `networking.firewall.allowedTCPPorts = with config.homeserver.ports; [ name1 name2 ];`
- Internal-only ports (e.g. Loki, Promtail) do not get a firewall entry.

---

## Step 3 — Module structure

Follow this order within the file:

```
{ config, pkgs, lib, ... }:

{
  # ── User/group (custom services only) ──────────────────────────────────────

  users.groups.X = { };
  users.users.X  = { ... };

  # ── Service ─────────────────────────────────────────────────────────────────

  services.X = { ... };

  # ── Systemd overrides (sandbox, EnvironmentFile) ────────────────────────────

  systemd.services.X.serviceConfig = ...;

  # ── Firewall ─────────────────────────────────────────────────────────────────

  networking.firewall.allowedTCPPorts = with config.homeserver.ports; [ ... ];
}
```

Section headers use this exact format:
```
  # ---------------------------------------------------------------------------
  # Service name — one-line description
  # ---------------------------------------------------------------------------
```

---

## Step 4 — After writing

1. Add the module to `flake.nix` in the correct group:
   - `nixos-stable/` modules go after `databases.nix`
   - `nixos-unstable/` modules go after `vaultwarden.nix`
   - `containers-podman/` modules go after `podman.nix`
   - `services/` modules go after `lan-apps.nix`

2. `sudo git -C /etc/nixos add <new-file>` — flakes only see tracked files.

3. Update `CLAUDE.md`:
   - Add a row to the services/ports table.
   - Add a row to the secrets table if applicable.

4. Update `check-secrets.sh` if the service has a secrets file.

5. Update `modules/nixos-stable/promtail.nix` — add the systemd unit name to the regex in the matching rules group:

   | Module folder | Rules group | What to update |
   |---|---|---|
   | `nixos-stable/` | `rulesNixosStable` | Add service name to the alternation regex |
   | `nixos-unstable/` | `rulesNixosUnstable` | Add service name to the alternation regex |
   | `containers-podman/` | `rulesContainers` | Only needed if unit name does NOT follow `podman-<name>.service` pattern |
   | `services/` | `rulesServices` | Add service name to the alternation regex |

   For `nixos-sys` services, add to the appropriate group: `rulesAcme`, `rulesNix`, `rulesNixosSys`, or `rulesRemote`.

   If the service name needs a clean label (e.g. stripping a prefix), add an explicit service name override rule in the same group, following the pattern used for `phpfpm-nextcloud` → `nextcloud`.

6. Run `sudo nixos-rebuild test --flake /etc/nixos#homeserver` before `switch`.
