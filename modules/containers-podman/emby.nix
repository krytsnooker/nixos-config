{ config, pkgs, lib, ... }:

{
  virtualisation.podman.enable = true;
  virtualisation.oci-containers.backend = "podman";

  virtualisation.oci-containers.containers.emby = {
    image = "emby/embyserver@sha256:3aafff933d3f28d23ed0bc201022abe71c0aa80deb17177566c726b9bbc686c6";
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
