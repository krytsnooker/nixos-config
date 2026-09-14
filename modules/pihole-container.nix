{ config, pkgs, lib, ... }:

{
  virtualisation.podman = {
    enable = true;
    dockerCompat = true;
  };

  virtualisation.oci-containers.backend = "podman";
  virtualisation.oci-containers.containers.pihole = {
    image = "pihole/pihole:2026.07.2";
    ports = [
      "53:53/tcp"
      "53:53/udp"
      # web UI on 8083 to avoid clashing with nginx on 80. NOTE: container's
      # internal webserver port came back as 8080 (not the image's usual 80)
      # after a Teleporter restore imported the old box's general settings,
      # which had it configured that way — mapping matches that.
      "8083:8080/tcp"
    ];
    environment = {
      TZ = config.time.timeZone;
      FTLCONF_dns_listeningMode = "ALL";
    };
    environmentFiles = [ "/var/lib/pihole/secrets.env" ];
    volumes = [
      "/var/lib/pihole/etc-pihole:/etc/pihole"
      "/var/lib/pihole/etc-dnsmasq.d:/etc/dnsmasq.d"
    ];
    extraOptions = [ "--cap-add=NET_ADMIN" ];
  };

  networking.firewall.allowedTCPPorts = [ 53 8083 ];
  networking.firewall.allowedUDPPorts = [ 53 ];

  systemd.tmpfiles.rules = [
    "d /var/lib/pihole/etc-pihole 0755 root root -"
    "d /var/lib/pihole/etc-dnsmasq.d 0755 root root -"
  ];
}
