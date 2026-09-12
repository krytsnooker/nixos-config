{ config, pkgs, lib, ... }:

{
  # --- Flakes ---
  nix.settings.experimental-features = [ "nix-command" "flakes" ];
  nix.gc = {
    automatic = true;
    dates = "weekly";
    options = "--delete-older-than 14d";
  };

  # --- Boot ---
  boot.loader.systemd-boot.enable = true;
  boot.loader.efi.canTouchEfiVariables = true;

  time.timeZone = "Australia/Sydney";

  i18n.defaultLocale = "en_US.UTF-8";
  console.keyMap = "us";

  # --- Networking basics (per-host hostname is set in hosts/<name>/configuration.nix) ---
  networking.networkmanager.enable = true;
  networking.firewall.enable = true;

  # --- User account ---
  # Matches your existing UID/GID (1000:1000) from the live box so DAS/share
  # file ownership carries over without a chown pass.
  users.users.kryt = {
    isNormalUser = true;
    uid = 1000;
    description = "kryt";
    extraGroups = [ "wheel" "networkmanager" "video" "audio" "render" "sambashare" ];
    # Set a password after first boot with: passwd kryt
    initialPassword = "changeme";
  };
  users.groups.kryt.gid = 1000;

  # Matches the live box's "sambashare" group (gid 125) — the CIFS mount to
  # the remote DAS share (see hosts/homeserver/configuration.nix) uses this
  # gid for file/dir permissions, so anything needing read/write there
  # (kryt, and Emby's service — see modules/emby.nix) must be a member.
  users.groups.sambashare.gid = 125;

  # --- Services that were running system-wide on the old box ---
  # Direct 1:1 equivalents of the plain systemd daemons from your service list.
  services.avahi = {
    enable = true;
    nssmdns4 = true;
  };
  hardware.bluetooth.enable = true;
  services.colord.enable = true;
  services.cron.enable = true;
  services.printing = {
    enable = true;
    browsing = true; # cups-browsed equivalent
  };
  services.fwupd.enable = true;
  services.irqbalance.enable = true;
  services.thermald.enable = true; # Intel-specific, worth keeping on both CPUs
  services.power-profiles-daemon.enable = true;
  services.udisks2.enable = true;
  services.upower.enable = true;
  security.rtkit.enable = true;
  security.polkit.enable = true;

  # Skipped intentionally vs. the old install (mainstream NixOS defaults cover
  # their purpose or they're not relevant to this hardware):
  #   - snapd / snap.*        -> not supported well on NixOS, replaced natively (see nextcloud.nix)
  #   - nxserver (NoMachine)  -> dropped in favor of xrdp (see xrdp.nix)
  #   - rsyslog               -> journald covers this by default
  #   - ModemManager          -> no cellular modem on this hardware
  #   - switcheroo-control    -> no hybrid graphics on this hardware
  #   - touchegg              -> no touchscreen/touchpad gesture needs on this hardware
  #   - kerneloops             -> crash-report submission daemon, not worth carrying over
  #   - wpa_supplicant (standalone) -> NetworkManager manages Wi-Fi directly
  # If you actually need any of these back, say so and I'll add them.

  environment.systemPackages = with pkgs; [
    vim
    git
    wget
    curl
    htop
    ripgrep
  ];

  system.stateVersion = "24.11";
}
