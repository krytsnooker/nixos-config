{ config, pkgs, lib, ... }:

{
  virtualisation.podman.enable = true;
  virtualisation.oci-containers.backend = "podman";

  virtualisation.oci-containers.containers.emby = {
    image = "emby/embyserver:latest";
    ports = [ "8096:8096/tcp" "8920:8920/tcp" ];
    volumes = [
      "/var/lib/emby/config:/config"
      "/mnt/server-pc/Media:/media"
    ];
    environment = {
      PUID = "0";
      PGID = "125";
    };
    extraOptions = [
      "--device=/dev/dri:/dev/dri"
    ];
  };

  systemd.tmpfiles.rules = [
    "d /var/lib/emby/config 0755 root root -"
  ];

  networking.firewall.allowedTCPPorts = [ 8096 8920 ];
}
