{ config, pkgs, lib, ... }:

let
  stateDir = "/var/lib/pihole";
  image    = "pihole/pihole@sha256:7c96327ecfb96dbc74b0a47d073dbef7d60526e0aa87519b2a2f7a0007cb5c88";
in
{
  # ---------------------------------------------------------------------------
  # Container
  # ---------------------------------------------------------------------------

  virtualisation.oci-containers.containers.pihole = {
    inherit image;
    ports = [
      "${toString config.homeserver.ports.dns}:${toString config.homeserver.ports.dns}/tcp"
      "${toString config.homeserver.ports.dns}:${toString config.homeserver.ports.dns}/udp"
      "${toString config.homeserver.ports.pihole}:${toString config.homeserver.ports.piholeInternal}/tcp"
    ];
    volumes = [
      "${stateDir}/etc-pihole:/etc/pihole"
      "${stateDir}/etc-dnsmasq.d:/etc/dnsmasq.d"
    ];
    environment = {
      TZ = config.time.timeZone;
      FTLCONF_dns_listeningMode = "ALL";
      FTLCONF_webserver_port    = toString config.homeserver.ports.piholeInternal;
    };
    environmentFiles = [ "${stateDir}/secrets.env" ];
    extraOptions     = [ "--cap-add=NET_ADMIN" ];
  };

  # ---------------------------------------------------------------------------
  # Storage
  # ---------------------------------------------------------------------------

  systemd.tmpfiles.rules = [
    "d ${stateDir}/etc-pihole    ${config.homeserver.podman.tmpfiles.mode} pihole pihole ${config.homeserver.podman.tmpfiles.age}"
    "d ${stateDir}/etc-dnsmasq.d ${config.homeserver.podman.tmpfiles.mode} pihole pihole ${config.homeserver.podman.tmpfiles.age}"
  ];

  # ---------------------------------------------------------------------------
  # Firewall
  # ---------------------------------------------------------------------------

  networking.firewall.allowedTCPPorts = with config.homeserver.ports; [ dns pihole ];
  networking.firewall.allowedUDPPorts = [ config.homeserver.ports.dns ];
}
