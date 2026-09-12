{ config, pkgs, lib, ... }:

{
  # Correction from an earlier assumption: this machine does NOT host the
  # SAS DAS locally. The live box's /etc/samba/smb.conf turned out to have
  # no custom shares at all (just Ubuntu's stock [printers]/[print$]) — the
  # actual DAS lives on a separate machine (192.168.0.110) and this box is
  # just a CIFS *client* of it (see hosts/homeserver/configuration.nix for
  # the mount). smbd/nmbd are kept enabled here only for printer-sharing
  # parity with the old setup; there's no data share to define.
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

  services.samba-wsdd.enable = true; # lets modern Windows discover it in Explorer

  # cifs-utils is needed client-side to mount the remote DAS share.
  environment.systemPackages = [ pkgs.cifs-utils ];
}
