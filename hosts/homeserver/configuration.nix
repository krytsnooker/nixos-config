{ config, pkgs, lib, ... }:

{
  imports = [
    ./hardware-configuration.nix # generated on-site by `nixos-generate-config --root /mnt`
  ];

  networking.hostName = "homeserver";

  networking.hosts = {
    "192.168.0.120" = [ "homeserver" ];
  };

  # i5-7500T / 8GB RAM — small safety-margin swap, no hibernation needed (24/7 box).
  swapDevices = [
    { device = "/swapfile"; size = 4096; } # MB
  ];

  # --- Remote DAS share (SMB/CIFS), matching the live box's /etc/fstab ---
  # The SAS DAS + HBA live on a SEPARATE machine (192.168.0.110), not this
  # box — this is a client mount only, same as the old setup.
  fileSystems."/mnt/server-pc" = {
    device = "//192.168.0.110/hsas01";
    fsType = "cifs";
    options = [
      "credentials=/home/kryt/.smbcredentials"
      "gid=125" # sambashare, see modules/common.nix
      "file_mode=0770"
      "dir_mode=0770"
      "iocharset=utf8"
      "_netdev"
      "x-systemd.automount"
      "nofail"
    ];
  };

  # Dedicated CIFS mount for Nextcloud user data.
  # uid= is derived from homeserver.nextcloudUid (declared in modules/nextcloud.nix)
  # so the service UID and the mount option stay in sync automatically.
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


  # --- Samba on this box itself ---
  # See modules/samba.nix — this machine doesn't serve any custom share
  # (confirmed from the live smb.conf), just kept for printer-sharing parity.
}
