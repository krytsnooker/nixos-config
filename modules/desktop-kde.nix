{ config, pkgs, lib, ... }:

{
  # Old box used lightdm + (presumably) a lighter desktop. Using SDDM + Plasma 6
  # here since that's the NixOS-native, best-supported pairing for KDE.
  # Swap to services.displayManager.lightdm.enable = true if you want to match
  # the old setup exactly instead.
  services.xserver.enable = true;
  services.displayManager.sddm.enable = true;
  services.desktopManager.plasma6.enable = true;

  services.xserver.xkb.layout = "us";

  # Default apps — edit this list freely.
  environment.systemPackages = with pkgs; [
    firefox
    vscode
  ];

  # Sound (Plasma 6 default stack)
  security.rtkit.enable = true;
  services.pipewire = {
    enable = true;
    alsa.enable = true;
    pulse.enable = true;
  };
}
