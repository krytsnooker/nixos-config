{ config, pkgs, lib, ... }:

let
  stateDir = "/var/lib/emby";
  image    = "emby/embyserver@sha256:3aafff933d3f28d23ed0bc201022abe71c0aa80deb17177566c726b9bbc686c6";
in
{
  # ---------------------------------------------------------------------------
  # User
  # ---------------------------------------------------------------------------

  users.users.emby.extraGroups = [ "video" "sambashare" ];

  # ---------------------------------------------------------------------------
  # Container
  # ---------------------------------------------------------------------------

  virtualisation.oci-containers.containers.emby = {
    inherit image;
    ports = [
      "${toString config.homeserver.ports.emby}:${toString config.homeserver.ports.emby}/tcp"
      "${toString config.homeserver.ports.embyHttps}:${toString config.homeserver.ports.embyHttps}/tcp"
    ];
    volumes = [
      "${stateDir}/config:/config"
      "${config.homeserver.paths.media}:/media"
    ];
    environment = {
      PUID = toString config.homeserver.uids.podman.emby;
      PGID = toString config.homeserver.gids.sambashare;
    };
    extraOptions = [ "--device=/dev/dri:/dev/dri" ];
  };

  # ---------------------------------------------------------------------------
  # Storage
  # ---------------------------------------------------------------------------

  systemd.tmpfiles.rules = [
    "d ${stateDir}/config ${config.homeserver.podman.tmpfiles.mode} emby emby ${config.homeserver.podman.tmpfiles.age}"
  ];

  # ---------------------------------------------------------------------------
  # Firewall
  # ---------------------------------------------------------------------------

  networking.firewall.allowedTCPPorts = with config.homeserver.ports; [ emby embyHttps ];
}
