# modules/services/minecraft.nix

Configures the NeoForge 1.20.1 Minecraft server and the mc-control web UI for starting, stopping, and adjusting server settings. Also defines a daily world backup job.

## nix-minecraft integration

`nix-minecraft.nixosModules.minecraft-servers` is imported via `imports` using `nix-minecraft` from `specialArgs`. This keeps the flake module list free of external module references — all flake inputs are wired at the module level.

The `nix-minecraft` overlay (added in `flake.nix`) makes NeoForge and other server packages available in `pkgs`.

## Minecraft server

| Setting | Value |
|---|---|
| Server name | `survival` (drives the systemd unit name `minecraft-server-survival.service`) |
| Data directory | `/var/lib/minecraft/survival` |
| Package | Custom `neoforge1201Package` wrapper (see below) |
| JVM opts | `-Xms1G -Xmx3G` |
| `autoStart` | `false` — managed by mc-control |
| `online-mode` | `false` (LAN-only; no Mojang auth) |
| `server-port` | `config.homeserver.ports.minecraft` (25565) |

### neoforge1201Package wrapper

`nix-minecraft` calls the package as `minecraft-server -Xms1G -Xmx3G`. The wrapper:
1. Writes the JVM args to `user_jvm_args.txt` (NeoForge's args file)
2. Runs `applySettingsScript` to merge `settings.json` onto `server.properties`
3. Executes `run.sh` via bash

### applySettingsScript

A Python script that reads `/var/lib/mc-control/settings.json` and applies its key/value pairs to `server.properties`. Called on every server start so mc-control-managed settings survive nix-minecraft's `ExecStartPre` regenerating `server.properties` from the Nix store. Keys in `READONLY = {"online-mode", "server-port"}` cannot be overridden.

## NeoForge installation service

`neoforge-1201-setup.service` is a oneshot that downloads and installs the NeoForge installer on first run. A marker file (`/.neoforge-1.20.1-47.1.106-installed`) prevents re-installation on subsequent starts. The Minecraft server unit depends on and waits for this service.

## mc-control

Flask/waitress web UI at port `config.homeserver.ports.mcControl` (5020). Runs as the `mcctl` system user.

### User and groups

| Account | Type | UID/GID | Purpose |
|---|---|---|---|
| `mcctl` | system user | `config.homeserver.uids.services.mcctl` (994) | Runs mc-control |
| `mcctl` | group | `config.homeserver.gids.mcctl` (993) | mc-control process group |
| `mcmods` | group | `config.homeserver.gids.mcmods` (980) | Shared group for mods directory |

`mcctl` is a member of `mcmods` and `minecraft` so it can read/write the mods directory and interact with the server.

### sudo rules

mc-control calls `/run/wrappers/bin/sudo` (the setuid wrapper) to control the systemd unit. The sudo rules allow `mcctl` to run these commands without a password:

- `systemctl start minecraft-server-survival.service`
- `systemctl stop minecraft-server-survival.service`
- `systemctl is-active minecraft-server-survival.service`
- `cat /var/lib/minecraft/survival/server.properties`
- `tee /var/lib/minecraft/survival/server.properties`

**Critical:** the rules reference `/run/current-system/sw/bin/systemctl` for the allowed command paths. mc-control itself must call `/run/wrappers/bin/sudo` (not `/run/current-system/sw/bin/sudo`) because only the wrapper has the setuid bit. If mc-control returns `{"status":"unknown"}`, verify `/run/wrappers/bin/sudo` is setuid root.

### Mods directory

`systemd.tmpfiles.rules` creates `/var/lib/minecraft/survival/mods` with mode `0775`, owner `minecraft`, group `mcmods`. This lets both the minecraft user and mcctl write mods.

## mc-backup

Daily rsync backup of `/var/lib/minecraft/survival/` to `config.homeserver.paths.backups/minecraft/<date>/`. Runs at 03:00 via a systemd timer with `Persistent = true`. Retains the 7 most recent daily snapshots and deletes older ones. Skips silently if `/mnt/server-pc` is not mounted.

## Firewall

Opens ports `minecraft` (25565) and `mcControl` (5020).
