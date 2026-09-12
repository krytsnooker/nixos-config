{ config, pkgs, lib, ... }:

{
  # Cross-platform remote desktop (Windows mstsc, Linux Remmina/FreeRDP).
  # Replaces NoMachine — see modules/desktop-kde.nix for the session it opens.
  services.xrdp = {
    enable = true;
    defaultWindowManager = "startplasma-x11";
    openFirewall = true;
  };
}
