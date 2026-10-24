{ config, pkgs, lib, ... }:

let
  stateDir = "/var/lib/audiobookshelf";
  image    = "ghcr.io/advplyr/audiobookshelf@sha256:e388e90e381ae3fa8660346612b2955f2c555ede81c9c286e2218bdf966b4de8";
  tz       = config.time.timeZone;
in
{
  # ---------------------------------------------------------------------------
  # User
  # ---------------------------------------------------------------------------

  users.users.audiobookshelf.extraGroups = [ "sambashare" ];

  # ---------------------------------------------------------------------------
  # Container
  # ---------------------------------------------------------------------------

  virtualisation.oci-containers.containers.audiobookshelf = {
    inherit image;
    ports = [ "${toString config.homeserver.ports.audiobookshelf}:80/tcp" ];
    volumes = [
      "${stateDir}/config:/config"
      "${stateDir}/metadata:/metadata"
      "${config.homeserver.paths.media}:/media"
    ];
    environment = {
      TZ   = tz;
      PUID = toString config.homeserver.uids.podman.audiobookshelf;
      PGID = toString config.homeserver.gids.sambashare;
    };
  };

  # ---------------------------------------------------------------------------
  # Storage
  # ---------------------------------------------------------------------------

  systemd.tmpfiles.rules = [
    "d ${stateDir}/config   ${config.homeserver.podman.tmpfiles.mode} audiobookshelf audiobookshelf ${config.homeserver.podman.tmpfiles.age}"
    "d ${stateDir}/metadata ${config.homeserver.podman.tmpfiles.mode} audiobookshelf audiobookshelf ${config.homeserver.podman.tmpfiles.age}"
  ];

  # ---------------------------------------------------------------------------
  # Firewall
  # ---------------------------------------------------------------------------

  networking.firewall.allowedTCPPorts = [ config.homeserver.ports.audiobookshelf ];
}
