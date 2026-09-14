{ config, pkgs, lib, ... }:

{
  security.acme = {
    acceptTerms = true;
    defaults.email = "krytsnooker@gmail.com"; # used only for Let's Encrypt renewal/expiry notices
  };

  services.nginx = {
    enable = true;
    recommendedProxySettings = true;
    recommendedTlsSettings = true;
    recommendedGzipSettings = true;

    # Ported from the old box's /etc/nginx/sites-available/emby.
    virtualHosts."intrentaka.com" = {
      serverAliases = [ "www.intrentaka.com" ];
      forceSSL = true;
      enableACME = true;

      locations."/" = {
        proxyPass = "http://127.0.0.1:8096";
        proxyWebsockets = true;
      };
    };

    # cloud.intrentaka.com (Nextcloud) vhost is created automatically by the
    # services.nextcloud module in modules/nextcloud.nix once
    # services.nextcloud.https = true — don't duplicate it here. The
    # client_max_body_size / HSTS tweaks from the old config are added there
    # via services.nginx.virtualHosts."cloud.intrentaka.com".extraConfig.
  };

  networking.firewall.allowedTCPPorts = [ 80 443 ];
}
