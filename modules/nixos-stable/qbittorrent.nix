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

  users.users.qbittorrent.extraGroups = [ "sambashare" ];
}
