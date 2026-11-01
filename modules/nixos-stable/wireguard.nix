{ config, pkgs, lib, ... }:

{
  # ---------------------------------------------------------------------------
  # WireGuard — remote access VPN server (10.100.0.0/24)
  # Server: 10.100.0.1  LAN: 192.168.0.0/24  UDP port 51820
  # Interface named wgsrv (not wg0) — vpn-confinement's qbittorrent netns
  # also creates a transient wg0 in the root namespace before moving it
  # into its own netns, and collides with a persistent wg0 here.
  # ---------------------------------------------------------------------------

  networking.wireguard.interfaces.wgsrv = {
    ips        = [ "10.100.0.1/24" ];
    listenPort = config.homeserver.ports.wireguard;

    # Generated once: wg genkey | sudo tee /var/lib/wireguard/private.key && sudo chmod 600 /var/lib/wireguard/private.key
    # Public key:     sudo wg pubkey < /var/lib/wireguard/private.key
    privateKeyFile = "/var/lib/wireguard/private.key";

    # MASQUERADE — allows full-tunnel clients (AllowedIPs = 0.0.0.0/0) to
    # reach the internet through the server's LAN interface.
    # Split-tunnel clients (AllowedIPs = 10.100.0.0/24, 192.168.0.0/24) don't
    # need this but it's harmless to have enabled.
    postSetup   = "${pkgs.iptables}/bin/iptables -t nat -A POSTROUTING -s 10.100.0.0/24 -o eno1 -j MASQUERADE";
    postShutdown = "${pkgs.iptables}/bin/iptables -t nat -D POSTROUTING -s 10.100.0.0/24 -o eno1 -j MASQUERADE";

    # Add one entry per client device.
    # { publicKey = "<wg pubkey output>"; allowedIPs = [ "10.100.0.X/32" ]; }
    peers = [
      {
        # Android
        publicKey  = "c81Hf3aewujU6a2BuxTPsnRvZpyVe0jWW9dOHWhnOUA=";
        allowedIPs = [ "10.100.0.2/32" ];
      }
    ];
  };

  # Private key directory — not world-readable
  systemd.tmpfiles.rules = [
    "d /var/lib/wireguard 0700 root root -"
  ];

  networking.firewall.allowedUDPPorts = with config.homeserver.ports; [ wireguard ];
}
