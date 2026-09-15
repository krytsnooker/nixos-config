{ config, pkgs, lib, ... }:

let
  brave-origin-nightly = pkgs.callPackage ../pkgs/brave-origin-nightly.nix { };
in
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
    brave-origin-nightly
    firefox
    vscode
    (python3.withPackages (ps: [ ps.pyqt6 ]))
  ];

  # chrome-sandbox must be setuid root for Chromium-based browsers to use
  # the kernel namespace sandbox (seccomp/namespaces).
  security.wrappers.brave-origin-nightly-sandbox = {
    source = "${brave-origin-nightly}/opt/brave.com/brave-origin-nightly/chrome-sandbox";
    owner = "root";
    group = "root";
    setuid = true;
  };

  # Sound (Plasma 6 default stack)
  security.rtkit.enable = true;
  services.pipewire = {
    enable = true;
    alsa.enable = true;
    pulse.enable = true;
  };
}
