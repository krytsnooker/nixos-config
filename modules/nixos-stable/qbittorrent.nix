{ config, pkgs, lib, ... }:

{
  # ---------------------------------------------------------------------------
  # qBittorrent — headless BitTorrent client with web UI, routed via VPN
  # ---------------------------------------------------------------------------

  services.qbittorrent = {
    enable         = true;
    webuiPort      = config.homeserver.ports.qbittorrent;
    torrentingPort = config.homeserver.ports.qbittorrentTorrent;
    openFirewall   = false;
    serverConfig   = {
      LegalNotice.Accepted = true;
    };
  };

  vpnNamespaces.wg.portMappings = [{
    from = config.homeserver.ports.qbittorrent;
    to   = config.homeserver.ports.qbittorrent;
  }];

  vpnNamespaces.wg.openVPNPorts = [{
    port     = config.homeserver.ports.qbittorrentTorrent;
    protocol = "both";
  }];

  systemd.services.qbittorrent.vpnConfinement = {
    enable       = true;
    vpnNamespace = "wg";
  };

  # qbittorrent explicitly requires PrivateTmp = false (upstream constraint)
  systemd.services.qbittorrent.serviceConfig =
    builtins.removeAttrs config.homeserver.sandbox [ "PrivateTmp" ];

  networking.firewall.allowedTCPPorts = with config.homeserver.ports; [ qbittorrent ];
}
