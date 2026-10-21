{ config, pkgs, lib, ... }:

{
  virtualisation.oci-containers.containers.audiobookshelf = {
    image = "ghcr.io/advplyr/audiobookshelf@sha256:e388e90e381ae3fa8660346612b2955f2c555ede81c9c286e2218bdf966b4de8";
    ports = [ "13378:80/tcp" ];
    volumes = [
      "/var/lib/audiobookshelf/config:/config"
      "/var/lib/audiobookshelf/metadata:/metadata"
      "/mnt/server-pc/Media:/media"
    ];
    environment = {
      TZ = "Europe/London";
    };
  };

  systemd.tmpfiles.rules = [
    "d /var/lib/audiobookshelf/config   0755 root root -"
    "d /var/lib/audiobookshelf/metadata 0755 root root -"
  ];

  networking.firewall.allowedTCPPorts = [ 13378 ];
}
