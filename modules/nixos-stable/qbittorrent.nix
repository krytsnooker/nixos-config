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
      Preferences.WebUI = {
        Username             = "admin";
        Password_PBKDF2      = "@ByteArray(9bpuuvQJ+CnrzcjzZFLmjg==:G0Lp4pKwSmQyNZUKAgMM/np2tYTa3m01wQjjIghs+dyFQCAonL+0rDOKbrvHY43Cx+i6DJ5EUXd6dafWbfvOFA==)";
        AlternativeUIEnabled = true;
        RootFolder           = "${pkgs.vuetorrent}/share/vuetorrent";
      };
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

  users.users.qbittorrent.extraGroups = [ "sambashare" ];

  systemd.tmpfiles.rules = [
    "d /var/lib/qBittorrent 0755 qbittorrent qbittorrent -"
  ];
}
