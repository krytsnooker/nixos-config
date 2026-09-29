{ config, pkgs, lib, ... }:

{
  services.xserver.enable = true;
  services.displayManager.sddm.enable = true;
  services.desktopManager.plasma6.enable = true;
  services.xserver.xkb.layout = "us";

  # ── Desktop hardware ──────────────────────────────────────────────────────
  hardware.bluetooth.enable = true;
  services.colord.enable = true;
  services.printing = {
    enable = true;
    browsing = true;
  };
  services.udisks2.enable = true;
  services.upower.enable = true;
  security.polkit.enable = true;

  # ── Sound ─────────────────────────────────────────────────────────────────
  security.rtkit.enable = true;
  services.pipewire = {
    enable = true;
    alsa.enable = true;
    pulse.enable = true;
  };

  # ── Apps ──────────────────────────────────────────────────────────────────
  environment.systemPackages = with pkgs; [
    brave-origin-nightly
    firefox
    vscode
    (python3.withPackages (ps: [ ps.pyqt6 ]))
  ];

  # chrome-sandbox must be setuid root for Chromium-based browsers to use
  # the kernel namespace sandbox (seccomp/namespaces).
  security.wrappers.brave-origin-nightly-sandbox = {
    source = "${pkgs.brave-origin-nightly}/opt/brave.com/brave-origin-nightly/chrome-sandbox";
    owner = "root";
    group = "root";
    setuid = true;
  };
}
