# configuration.nix

The base system configuration. Sets machine identity, boot, networking, user accounts, desktop environment, remote access, and CIFS mounts. Everything here is host-specific and does not belong in a module.

## Sections

### Identity

- Hostname: `homeserver`
- Static hosts entry: `192.168.0.120 → homeserver`, `192.168.0.123 → bc250`
- Timezone: `Australia/Sydney`
- `system.configurationRevision` stamped from `self.rev` (the flake's git commit hash, or `"dirty"` on an unclean tree)

### Boot

- systemd-boot EFI loader
- 4 GiB swapfile at `/swapfile` (the hardware `swapDevices = []` is overridden here)
- Nix build parallelism capped: `cores = 2`, `max-jobs = 1` — prevents OOM on 8 GiB RAM during Rust source builds
- Weekly GC: deletes generations older than 14 days

### Users

`kryt` (UID/GID 1000) is the only non-system user. The UID is pinned because the DAS CIFS share files are owned by 1000; changing it would require a full chown pass on the share.

`sambashare` GID is set here from `config.homeserver.gids.sambashare` so it is consistent with every other consumer of that group.

### Desktop

KDE Plasma 6 on X11 via SDDM. Bluetooth, printing (CUPS), PipeWire (ALSA + PulseAudio compat), upower, udisks2, polkit, and colord are all enabled. KWallet is disabled via `environment.etc."xdg/kwalletrc"` — it has no purpose on a server desktop and prompts unnecessarily after XRDP login.

### Remote desktop (XRDP)

XRDP with a custom start script that:
1. Sets `KWIN_COMPOSE=N` to disable compositing (XRDP has no hardware GL; compositing crashes kwin)
2. Disables the screen lock via `kwriteconfig6` (lock screen is unresponsive over XRDP without compositing)
3. Launches `startplasma-x11`

### Samba

`smbd`/`nmbd` enabled for printer-sharing compatibility only — this machine has no data shares. The DAS is a client mount, not a share. `samba-wsdd` allows Windows network discovery.

### CIFS mounts

Two mounts from the DAS at `192.168.0.110`:

| Mount point | Share | Notes |
|---|---|---|
| `/mnt/server-pc` | `//192.168.0.110/hsas01` | General access; GID pinned to `sambashare` (125) |
| `/var/lib/nextcloud/data` | `//192.168.0.110/hsas01/Nextcloud/data` | UID pinned to `uids.unstable.nextcloud`; Nextcloud service depends on this mount |

Both use `x-systemd.automount` (lazy-mount on first access) and `nofail` (boot succeeds if the DAS is offline).
