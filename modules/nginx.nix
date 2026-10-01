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
    logError = "/var/log/nginx/error.log";
    commonHttpConfig = "access_log /var/log/nginx/access.log;";
    appendHttpConfig = "limit_req_zone \$binary_remote_addr zone=emby_auth:10m rate=5r/s;";

    # Ported from the old box's /etc/nginx/sites-available/emby.
    virtualHosts."intrentaka.com" = {
      serverAliases = [ "www.intrentaka.com" ];
      forceSSL = true;
      enableACME = true;

      locations."^~ /swagger" = {
        extraConfig = "return 404;";
      };

      locations."~ ^/(emby/)?Users/AuthenticateByName$" = {
        proxyPass = "http://127.0.0.1:8096";
        proxyWebsockets = true;
        extraConfig = ''
          limit_req zone=emby_auth burst=10 nodelay;
          limit_req_status 429;
          add_header Strict-Transport-Security "max-age=15552000; includeSubDomains" always;
          add_header X-Frame-Options "SAMEORIGIN" always;
          add_header X-Content-Type-Options "nosniff" always;
          add_header Referrer-Policy "strict-origin-when-cross-origin" always;
          add_header Permissions-Policy "geolocation=(), microphone=(), camera=()" always;
          proxy_hide_header Access-Control-Allow-Origin;
          proxy_hide_header Access-Control-Allow-Credentials;
          proxy_hide_header Access-Control-Allow-Private-Network;
          add_header Access-Control-Allow-Origin "*" always;
        '';
      };

      locations."/" = {
        proxyPass = "http://127.0.0.1:8096";
        proxyWebsockets = true;
        extraConfig = ''
          add_header Strict-Transport-Security "max-age=15552000; includeSubDomains" always;
          add_header X-Frame-Options "SAMEORIGIN" always;
          add_header X-Content-Type-Options "nosniff" always;
          add_header Referrer-Policy "strict-origin-when-cross-origin" always;
          add_header Permissions-Policy "geolocation=(), microphone=(), camera=()" always;
          proxy_hide_header Access-Control-Allow-Origin;
          proxy_hide_header Access-Control-Allow-Credentials;
          proxy_hide_header Access-Control-Allow-Private-Network;
          add_header Access-Control-Allow-Origin "*" always;
        '';
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
