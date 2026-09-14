# homeserver NixOS config

HP ProDesk G4 800 · i5-7500T · 8GB RAM · 256GB SSD (OS) · NixOS 25.11

Media and user data live on a Windows 10 machine at `192.168.0.110` with a
SAS DAS attached via HBA. This box mounts it over CIFS at `/mnt/server-pc`.

---

## Hardware

| Partition | UUID | Filesystem | Mount |
|---|---|---|---|
| nvme0n1p1 | A745-730E | vfat FAT32 | /boot (ESP) |
| nvme0n1p2 | f2ba3164-49c0-4f90-bea6-8c7ada01d308 | btrfs | / |
| nvme0n1p3 | 8034dca1-d1fd-41fd-bc45-941c2e1ef74c | btrfs | /home |

Swap: 4 GB swapfile at `/swapfile`.

---

## Repository layout

```
flake.nix                    — single nixosConfigurations.homeserver
hosts/homeserver/            — hardware config + per-host settings
modules/                     — NixOS modules (all loaded by every build)
pkgs/                        — custom packages (brave-origin-nightly)
apps/                        — source for custom LAN apps (canonical copy)
```

---

## Rebuilding

```bash
# Apply config changes
sudo nixos-rebuild switch --flake /etc/nixos#homeserver

# Test without making it the boot default
sudo nixos-rebuild test --flake /etc/nixos#homeserver

# Set as next boot without touching the running system
sudo nixos-rebuild boot --flake /etc/nixos#homeserver

# Roll back to previous generation
sudo nixos-rebuild switch --rollback
```

Flakes only see git-tracked files — stage changes before rebuilding:
```bash
git add <file>
```

## Known-good snapshots

Tag the current state whenever things are stable:
```bash
git tag working-YYYY-MM-DD
```

To restore:
```bash
git checkout working-YYYY-MM-DD
sudo nixos-rebuild switch --flake /etc/nixos#homeserver
```

---

## Services

| Service | Port | URL | Notes |
|---|---|---|---|
| homepage | 5000 | http://homeserver:5000 | Static, served by nginx |
| typing-tutor | 3000 | http://homeserver:3000 | Node/Express |
| math-tutor | 3001 | http://homeserver:3001 | Node/Express |
| lan-pastebin | 5001 | http://homeserver:5001 | Flask/waitress |
| music-info | 5010 | http://homeserver:5010 | Flask/waitress |
| Emby | 8096 / 8920 | https://intrentaka.com | Podman container |
| Nextcloud | 443 | https://cloud.intrentaka.com | Native NixOS service |
| Pi-hole | 8083 | http://homeserver:8083 | Podman container |
| Minecraft | 25565 | LAN only | manual start, not on boot |

```bash
systemctl --failed                 # anything that didn't start
journalctl -u <service> -f        # live logs for a service
```

---

## Secrets (never stored in this repo)

### `/home/kryt/.smbcredentials` — chmod 600, owned kryt:kryt
Required for the CIFS mount. Copy machine-to-machine (scp/USB), never paste
into chat or logs.
```
username=<user>
password=<password>
domain=WORKGROUP
```

### `/var/lib/music-info/secrets.env` — chmod 600, owned music-info:music-info
```
DISCOGS_TOKEN=<your Discogs token>
EMBY_API_KEY=<your Emby API key>
EMBY_BASE_URL=http://localhost:8096
EMBY_EXTERNAL_URL=https://intrentaka.com
```
`MUSIC_PATH` is set in the systemd unit in `modules/lan-apps.nix`, not here.

### `/var/lib/pihole/secrets.env` — chmod 600, owned root:root
```
WEBPASSWORD=<your Pi-hole admin password>
```

### `/var/lib/nextcloud-admin-pass` — chmod 600, owned root:root
Single line: the Nextcloud admin password.

---

## Music library scanner

Reads metadata from the directory structure (`Artist/Album/NN - Title.mp3`)
and MP3/FLAC duration headers using 16 parallel CIFS workers. Scans ~27,000
tracks in ~13 minutes.

Rescan after adding music to the DAS:
```bash
sudo systemctl start music-scanner
journalctl -fu music-scanner       # live progress
```

Full wipe + rebuild of the `tracks` table on every run.

---

## SSL / external access

Let's Encrypt via NixOS ACME (auto-renews). Port 80 and 443 must be
forwarded from the router to this machine.

- `intrentaka.com` / `www.intrentaka.com` → Emby (`localhost:8096`)
- `cloud.intrentaka.com` → Nextcloud


---

## Minecraft

```bash
sudo systemctl start minecraft-server-survival
sudo systemctl stop minecraft-server-survival
```

`autoStart = false` — does not start at boot. Also controllable from the
homepage Minecraft card via the `mc-control` API (port 5020).

`online-mode = false` for LAN clients without Mojang accounts. Do not expose
port 25565 externally without re-enabling online mode first.

**NixOS gotcha:** `mc-control` uses `/run/wrappers/bin/sudo` (the setuid
wrapper NixOS creates at boot), not `/run/current-system/sw/bin/sudo` which
lacks the setuid bit. If mc-control ever returns `{"status":"unknown"}` after
a reinstall, verify `/run/wrappers/bin/sudo` exists and is setuid root.

---

## Fresh install

1. Partition 256GB SSD: 512MB EFI (vfat) + btrfs root + btrfs `/home`
2. Mount at `/mnt`, `/mnt/boot`, `/mnt/home`
3. `nixos-generate-config --root /mnt` → overwrites `hosts/homeserver/hardware-configuration.nix`
4. Clone this repo to `/mnt/etc/nixos/`
5. Create all secrets files listed above
6. `nixos-install --flake /mnt/etc/nixos#homeserver`
7. Reboot → `passwd kryt` → place `.smbcredentials`
8. Verify CIFS mount: `systemctl status 'mnt-server\x2dpc.mount'`
9. Run music scanner: `sudo systemctl start music-scanner`
10. Restore Nextcloud and Pi-hole from DAS backups at `/mnt/server-pc/Backups/`
