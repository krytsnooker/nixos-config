{ config, pkgs, lib, ... }:

{
  virtualisation.podman = {
    enable = true;
    dockerCompat = true;
  };

  virtualisation.oci-containers.backend = "podman";
  virtualisation.oci-containers.containers.pihole = {
    image = "pihole/pihole:latest";
    ports = [
      "53:53/tcp"
      "53:53/udp"
      "8083:80/tcp" # web UI on 8083 to avoid clashing with nginx on 80
    ];
    environment = {
      TZ = config.time.timeZone;
      # WEBPASSWORD = "changeme"; # TODO: set a real admin password (or via environmentFiles below)
    };
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
