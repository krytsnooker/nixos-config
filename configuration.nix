{ config, pkgs, lib, self, ... }:

{
  # ── Identity ──────────────────────────────────────────────────────────────
  system.configurationRevision = self.rev or "dirty";
  networking.hostName = "homeserver";
  networking.hosts."192.168.0.120" = [ "homeserver" ];
  time.timeZone = "Australia/Sydney";
  i18n.defaultLocale = "en_US.UTF-8";
  console.keyMap = "us";

  # ── Boot ──────────────────────────────────────────────────────────────────
  boot.loader.systemd-boot.enable = true;
  boot.loader.efi.canTouchEfiVariables = true;
  # i5-7500T / 8GB RAM — small safety-margin swap, no hibernation needed.
  swapDevices = [{ device = "/swapfile"; size = 4096; }];

  # ── Networking ────────────────────────────────────────────────────────────
  networking.networkmanager.enable = true;
  networking.firewall.enable = true;
  networking.hosts."192.168.0.123" = [ "bc250" ];

  # ── Users ─────────────────────────────────────────────────────────────────
  # UID 1000:1000 pinned to match DAS file ownership — never change without
  # running a chown pass on the share.
  users.users.kryt = {
    isNormalUser = true;
    uid = 1000;
    description = "kryt";
    extraGroups = [ "wheel" "networkmanager" "video" "audio" "render" "sambashare" ];
  };
  users.groups.kryt.gid = 1000;

  # GID 125 pinned — CIFS mounts, Emby (PGID=125), and music-info all depend
  # on this value. Change here and everything else breaks.
  users.groups.sambashare.gid = 125;

  # ── Nix ───────────────────────────────────────────────────────────────────
  nix.settings.experimental-features = [ "nix-command" "flakes" ];
  nix.settings.cores = 2;        # limit per-build parallelism; Rust source builds OOM at 4 cores on 8 GiB
  nix.settings.max-jobs = 1;     # one build at a time to cap peak RAM usage
  nix.gc = {
    automatic = true;
    dates = "weekly";
    options = "--delete-older-than 14d";
  };

  # ── System services ───────────────────────────────────────────────────────
  services.avahi = { enable = true; nssmdns4 = true; };
  services.cron.enable = true;
  services.fwupd.enable = true;
  services.irqbalance.enable = true;
  services.thermald.enable = true;
  services.power-profiles-daemon.enable = true;

  # ── Desktop ───────────────────────────────────────────────────────────────
  services.xserver.enable = true;
  services.displayManager.sddm.enable = true;
  services.desktopManager.plasma6.enable = true;
  services.xserver.xkb.layout = "us";

  hardware.bluetooth.enable = true;
  services.colord.enable = true;
  services.printing = { enable = true; browsing = true; };
  services.udisks2.enable = true;
  services.upower.enable = true;
  security.polkit.enable = true;

  security.rtkit.enable = true;
  services.pipewire = {
    enable = true;
    alsa.enable = true;
    pulse.enable = true;
  };

  environment.systemPackages = with pkgs; [
    brave-origin-nightly
    firefox
    vscode
    (python3.withPackages (ps: [ ps.pyqt6 ]))
    vim
    git
    wget
    curl
    htop
    ripgrep
    claude-code
    go
    cifs-utils
  ];

  # chrome-sandbox must be setuid root for Chromium-based browsers to use
  # the kernel namespace sandbox (seccomp/namespaces).
  security.wrappers.brave-origin-nightly-sandbox = {
    source = "${pkgs.brave-origin-nightly}/opt/brave.com/brave-origin-nightly/chrome-sandbox";
    owner = "root";
    group = "root";
    setuid = true;
  };

  # ── KWallet ───────────────────────────────────────────────────────────────
  environment.etc."xdg/kwalletrc".text = ''
    [Wallet]
    Enabled=false
  '';

  # ── Remote desktop ────────────────────────────────────────────────────────
  # XRDP has no hardware GL — kwin compositing crashes the window manager,
  # which leaves the lock screen displayed but unresponsive. Disable both.
  services.xrdp = {
    enable = true;
    defaultWindowManager = toString (pkgs.writeShellScript "start-plasma-xrdp" ''
      export KWIN_COMPOSE=N
      ${pkgs.kdePackages.kconfig}/bin/kwriteconfig6 \
        --file kscreenlockerrc --group Daemon --key Autolock false
      exec ${pkgs.kdePackages.plasma-workspace}/bin/startplasma-x11
    '');
    openFirewall = true;
  };

  # ── Samba ─────────────────────────────────────────────────────────────────
  # smbd/nmbd kept for printer-sharing parity — this machine has no data shares.
  # The DAS at 192.168.0.110 is a client mount only (see CIFS mounts below).
  services.samba = {
    enable = true;
    openFirewall = true;
    settings = {
      global = {
        workgroup = "WORKGROUP";
        "server string" = config.networking.hostName;
        security = "user";
        "map to guest" = "bad user";
      };
    };
  };
  services.samba-wsdd.enable = true;

  # ── CIFS mounts ───────────────────────────────────────────────────────────
  # DAS lives on a separate machine (192.168.0.110) — this is a client mount.
  fileSystems."/mnt/server-pc" = {
    device = "//192.168.0.110/hsas01";
    fsType = "cifs";
    options = [
      "credentials=/home/kryt/.smbcredentials"
      "gid=125"
      "file_mode=0770"
      "dir_mode=0770"
      "iocharset=utf8"
      "_netdev"
      "x-systemd.automount"
      "nofail"
    ];
  };

  # uid= kept in sync with homeserver.nextcloudUid declared in nextcloud.nix.
  fileSystems."/var/lib/nextcloud/data" = {
    device = "//192.168.0.110/hsas01/Nextcloud/data";
    fsType = "cifs";
    options = [
      "credentials=/home/kryt/.smbcredentials"
      "uid=${toString config.homeserver.nextcloudUid}"
      "gid=125"
      "file_mode=0660"
      "dir_mode=0770"
      "iocharset=utf8"
      "_netdev"
      "x-systemd.automount"
      "nofail"
    ];
  };

  system.stateVersion = "24.11";
}
