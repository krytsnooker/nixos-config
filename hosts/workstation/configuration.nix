{ config, pkgs, lib, ... }:

{
  imports = [
    ./hardware-configuration.nix # generated on-site by `nixos-generate-config --root /mnt`
  ];

  networking.hostName = "workstation";

  # i7-12700K / 32GB RAM — plenty of headroom, swap is just a safety margin.
  swapDevices = [
    { device = "/swapfile"; size = 8192; } # MB
  ];

  # This box only exists to validate the shared modules before they go on
  # the real target. Nextcloud/Emby/Pi-hole will still start here, but
  # there's no DAS attached — their data dirs just live on local disk.
}
