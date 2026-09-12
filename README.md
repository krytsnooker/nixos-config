# NixOS migration: workstation test -> homeserver target

## Layout

- `flake.nix` — defines two hosts: `workstation` (test) and `homeserver` (real target)
- `hosts/<name>/configuration.nix` — per-host settings (hostname, swap, disks)
- `hosts/<name>/hardware-configuration.nix` — **placeholder**, gets overwritten by `nixos-generate-config` during install
- `modules/*.nix` — shared services, used by both hosts
- `apps/*` — the reviewed/fixed source for the 5 custom LAN tools (homepage, lan_pastebin, math-tutor, music-info-project, typing-tutor) plus the new `mc-control` API. This is the canonical copy the Nix config actually builds from — `modules/lan-apps.nix` and `modules/minecraft.nix` reference these paths directly. The originals under `homeserverprojects/projects/` were kept in sync for the files that got fixed, but treat `nixos-config/apps/` as the source of truth going forward.

**Important**: this directory needs to be a git repo before `nixos-install --flake` or `nixos-rebuild --flake` will work — flakes only see git-tracked files. Before installing:
```bash
git init
git add .
git commit -m "initial config"
```
Re-run `git add .` after any future edit here (even uncommitted/staged is enough for flakes to see it, but commit properly once things are working).

## Install order

### 1. Workstation (test) — i7-12700K, 32GB RAM

1. Boot the NixOS installer USB on the workstation.
2. Partition the new SSD:
   - EFI system partition (~512MB, vfat)
   - btrfs root partition
   - btrfs /home partition (you asked for these separate)
   - No encryption (per your answer)
3. Format and mount at `/mnt`, `/mnt/boot`, `/mnt/home`.
4. `nixos-generate-config --root /mnt` — this **replaces**
   `hosts/workstation/hardware-configuration.nix` with the real one for this
   machine. Copy that generated file over the placeholder.
5. Copy this whole `nixos-config` folder to `/mnt/etc/nixos/` (or clone it
   from wherever you push it — GitHub, a USB stick, etc).
6. `nixos-install --flake /mnt/etc/nixos#workstation`
7. Reboot, log in as `kryt` / `changeme`, immediately run `passwd kryt`.
8. Validate everything: KDE session, xrdp from another machine, Nextcloud
   setup wizard, Emby, Pi-hole web UI (port 8083), homepage dashboard (port
   8082), Samba share visibility from a Windows PC.

### 2. Homeserver (real target) — HP ProDesk G4 800, i5-7500T, 8GB RAM

The SAS DAS is **not** local to this box — it's on a separate machine
(192.168.0.110) and this box just mounts it over CIFS, same as before. So
there's no local data disk to preserve/reformat here; the 256GB SSD is just
the OS disk.

Before wiping the old box, copy elsewhere (don't just leave it in place):
- `/home/kryt/.smbcredentials` — copy this file directly to the new machine
  (scp/USB stick), never paste its contents anywhere. It's what lets the new
  box authenticate to `//192.168.0.110/hsas01`.
- Nextcloud data + database — see the Nextcloud section below, this needs
  its own careful migration, not a first-boot afterthought.
- The 4 custom "homebrew" app project folders (pending, you're sending these separately).
- Any mariadb/postgresql dumps for data not otherwise reproducible.

Then:
1. Install the 256GB SSD as the OS disk, same partition scheme as the
   workstation (EFI + btrfs root + btrfs /home, no encryption).
2. Boot the installer, partition/format/mount the 256GB SSD.
3. `nixos-generate-config --root /mnt`, copy over
   `hosts/homeserver/hardware-configuration.nix`.
4. `nixos-install --flake /mnt/etc/nixos#homeserver`
5. Boot, place `.smbcredentials` at `/home/kryt/.smbcredentials`
   (`chmod 600`, `chown kryt:kryt`), then check the mount came up:
   `systemctl status mnt-server\\x2dpc.mount` and `df -h`.
6. Verify Samba/printer sharing is visible again from your other machines.

## What the live server dump changed vs. my original guesses

- **Storage architecture was wrong in the first draft**: I'd assumed the
  ProDesk itself hosted the SAS DAS and shared it out. The actual
  `/etc/fstab` and `/etc/samba/smb.conf` show the opposite — this box is a
  CIFS *client* of `//192.168.0.110/hsas01` (a separate machine owns the
  DAS/HBA), and its own Samba has no custom shares at all. Fixed in
  `modules/samba.nix` and `hosts/homeserver/configuration.nix`.
- **Username/UID**: now `kryt` (uid/gid 1000), matching the live box exactly
  so file ownership on the CIFS mount just works — was a generic `admin`
  placeholder before.
- **Nextcloud uses MySQL, not Postgres** (confirmed via `occ config:list`) —
  fixed in `modules/nextcloud.nix`. Postgres on the old box is presumably for
  something else (possibly a stale/unused `invidious` install per your
  answer) — not needed for Nextcloud.
- **Nextcloud uses Redis** for locking/caching — added
  `services.nextcloud.configureRedis = true`.
- **Nextcloud is on version 34.0.3** — this is the big one. Nextcloud
  refuses to open a data directory with an older server version than what
  wrote it (no downgrades). `flake.nix`'s nixpkgs pin was bumped to
  `nixos-25.11`, but **you must verify** that revision actually ships a
  nextcloud package ≥ 34.x before attempting the data import — check
  search.nixos.org or `nix search nixpkgs nextcloud` on the workstation. If
  it doesn't, point the flake at a newer input instead.
- **Emby hardware transcoding**: `/dev/dri` (card0 + renderD128) confirmed
  present on the live box, so QuickSync is plausible. Added
  `hardware.graphics` + `intel-media-driver` and put Emby's service in the
  `video`/`render`/`sambashare` groups in `modules/emby.nix`. Double check
  Emby's Playback settings after migrating to confirm hardware accel is
  actually toggled on.
- **Emby ran as a dedicated user (`mbsvr`)**, a member of the `sambashare`
  group — replicated via `sambashare` group membership on Emby's systemd
  service rather than recreating that exact username.
- **Cron jobs are all standard Debian/Pi-hole/certbot boilerplate** — nothing
  custom to port. Pi-hole's gravity-update/log-flush cron jobs already run
  *inside* the official container image, so the container setup in
  `modules/pihole-container.nix` needs no changes for this.
- **`steam` and `invidious` accounts are stale** (per your answer) — ignored,
  not carried into the new config.

## Still open

- **`services.emby`** module option names: not 100% certain these match the
  nixpkgs revision this flake ends up pinned to. Check search.nixos.org
  before relying on them.
- **nix-minecraft's `autoStart` option and `paperServers.paper-1_19_4`
  package attribute** — community flake, verify on the workstation (see
  Minecraft section below).
- **Timezone**: set to UTC as a placeholder in `modules/common.nix` —
  change `time.timeZone`.
- **Pi-hole password**: set a real `WEBPASSWORD` in
  `modules/pihole-container.nix` (currently commented out, meaning
  first-boot will generate a random one — check container logs for it).

## LAN apps (homepage, lan_pastebin, math-tutor, music-info-project, typing-tutor)

Packaged in `modules/lan-apps.nix`, source in `apps/*`. All run as
`DynamicUser` systemd services except `music-info` (needs a fixed user — see
below) — each gets its own isolated `/var/lib/<name>` for its database/logs,
completely separate from its read-only code in the Nix store.

| App | Port | Notes |
|---|---|---|
| homepage | 5000 | Static files, served directly by nginx — no backend process |
| typing-tutor | 3000 | Node/Express |
| math-tutor | 3001 | Node/Express |
| lan_pastebin | 5001 | Flask + waitress |
| music-info-project | 5010 | Flask + waitress; fixed `music-info` system user (not DynamicUser) so it can share `/var/lib/music-info` with the scanner and hold `sambashare` group membership for the CIFS mount |
| mc-control | 5020 | See Minecraft section |

**Before first boot**, create `/var/lib/music-info/secrets.env` (root-owned,
`chmod 600`) with:
```
DISCOGS_TOKEN=your-token-here
EMBY_API_KEY=your-emby-api-key-here
EMBY_BASE_URL=http://localhost:8096
EMBY_EXTERNAL_URL=https://intrentaka.com
MUSIC_PATH=/mnt/server-pc/Media/Music
```
(Rotate the Discogs token and Emby API key first — the old ones were pasted
into this chat session and shouldn't be reused.)

**Music library rescans are manual**: after adding new music to the DAS, run
`sudo systemctl start music-scanner.service`. It's a oneshot, not a timer —
check `journalctl -u music-scanner` for progress/errors. This does a full
wipe + rebuild of the `tracks` table (by design, see `music_scanner.py`'s
header comment) — **this file is a reconstruction**, the original scanner
was lost (confirmed identical-to-database.py on the live server too), so
verify it against your actual library before trusting it, per the earlier
discussion.

## Minecraft (Paper 1.19.4)

Packaged in `modules/minecraft.nix`, using the community
[nix-minecraft](https://github.com/Infinidoge/nix-minecraft) flake since
mainline nixpkgs only ships one current Minecraft version and 1.19.4 needs
pinning. **Verify `pkgs.paperServers.paper-1_19_4` and the `autoStart`
option still exist under that name** on the workstation before relying on
this — community flake APIs shift more than mainline nixpkgs.

- `online-mode = false` — required for offline/cracked-style clients (e.g.
  connecting via PollyMC without a Mojang account). This is safe specifically
  *because* the server is LAN-only (port 25565 is never reverse-proxied or
  forwarded on the router) — don't flip this setup to internet-facing
  without also turning `online-mode` back on, since offline-mode has no
  identity verification at all.
- **On-demand, not always-on**: `autoStart = false` means the systemd unit
  (`minecraft-server-survival.service`) exists but isn't started at boot.
  Start/stop it from the homepage's Minecraft card, which calls the tiny
  `mc-control` API (port 5020, source in `apps/mc-control`).
- **Security model for mc-control**: it runs as a dedicated `mcctl` system
  user whose *only* elevated permission is a `NOPASSWD` sudo rule for three
  exact commands — `systemctl start/stop/is-active` on that one unit,
  nothing else (see `security.sudo.extraRules` in `modules/minecraft.nix`).
  It's not root, not general sudo, and can't be used to control any other
  service. CORS is wide open on its API (matches the no-auth trust model of
  every other tool on this LAN) — don't expose port 5020 beyond the LAN.
- **Heap capped at `-Xmx3G`** (see the earlier RAM budget discussion) — if
  you add mods/plugins that need more, bump this deliberately rather than
  letting it grow unbounded on an 8GB box that's also running Emby/Nextcloud.

## SSL / external access (Emby + Nextcloud)

Ported from the old box's `/etc/nginx/sites-available/emby` and `/nextcloud`:

- `intrentaka.com` + `www.intrentaka.com` → Emby (`127.0.0.1:8096`, websockets
  on) — defined in `modules/nginx.nix`.
- `cloud.intrentaka.com` → Nextcloud — the `services.nextcloud` module
  creates this vhost itself; `modules/nextcloud.nix` just extends it with the
  old config's `client_max_body_size 512M` and HSTS header.
- Both use **native NixOS ACME** (`security.acme` + `enableACME`), matching
  what's actually live on the old box today (certbot/Let's Encrypt at
  `/etc/letsencrypt/live/intrentaka.com-0001/...`) — NixOS requests and
  renews the cert itself, nothing to copy over manually.
- The **DigiCert/PFX files** in the old `/etc/nginx/ssl/intrentaka.com/`
  folder are not used by either live site config (the Nextcloud config even
  has that path commented out in favor of Let's Encrypt) — they look like a
  leftover from an earlier attempt and aren't referenced anywhere in this
  config. Say so if that's wrong and there's a reason to keep them.
- ACME needs port 80 reachable from the internet for the HTTP-01 challenge
  (same as certbot needed) — make sure your router's port-forward for 80/443
  points at whichever box (workstation during testing, then homeserver)
  is live at the time.

## Useful commands once installed

```bash
# Rebuild after editing configs
sudo nixos-rebuild switch --flake /etc/nixos#workstation   # or #homeserver

# Test a config without making it the boot default
sudo nixos-rebuild test --flake /etc/nixos#workstation

# Roll back if something breaks
sudo nixos-rebuild switch --rollback
```
