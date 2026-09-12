{ config, pkgs, lib, ... }:

{
  # NOTE: verify `services.emby` still exists under that name in the nixpkgs
  # revision this flake is pinned to (search.nixos.org) before relying on it —
  # confirm this on the workstation test run first.
  services.emby = {
    enable = true;
    # dataDir defaults to /var/lib/emby
  };

  # Hardware transcoding (Intel QuickSync) — the live server has /dev/dri
  # (card0 + renderD128) present, and the i5-7500T's iGPU supports QSV, so
  # this is worth having even though I couldn't confirm Emby's encoding.xml
  # had it toggled on. Check Emby's dashboard > Playback settings after
  # migrating and enable "Hardware acceleration" > QuickSync if it isn't on.
  hardware.graphics = {
    enable = true;
    extraPackages = with pkgs; [ intel-media-driver ];
  };
  # "sambashare" (gid 125) is required to read/write the CIFS mount to the
  # remote DAS (see hosts/homeserver/configuration.nix) — the live box ran
  # Emby as user "mbsvr", a member of that group, for the same reason.
  systemd.services.emby.serviceConfig.SupplementaryGroups = [ "video" "render" "sambashare" ];

  networking.firewall.allowedTCPPorts = [ 8096 8920 ];

  # Media lives on the remote DAS at //192.168.0.110/hsas01, mounted at
  # /mnt/server-pc (see hosts/homeserver/configuration.nix) — same path the
  # old box used. Point Emby's library paths at /mnt/server-pc/... via its
  # web UI after first boot; no local storage needed on this machine for media.
}
