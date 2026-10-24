{ config, pkgs, lib, ... }:

let
  securityHeaders = ''
    add_header Strict-Transport-Security "max-age=15552000; includeSubDomains" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Permissions-Policy "geolocation=(), microphone=(), camera=()" always;
  '';
  corsHeaders = ''
    proxy_hide_header Access-Control-Allow-Origin;
    proxy_hide_header Access-Control-Allow-Credentials;
    proxy_hide_header Access-Control-Allow-Private-Network;
    add_header Access-Control-Allow-Origin "*" always;
  '';
in
{
  # ---------------------------------------------------------------------------
  # ACME
  # ---------------------------------------------------------------------------

  security.acme = {
    acceptTerms = true;
    defaults.email = "krytsnooker@gmail.com";
  };

  # ---------------------------------------------------------------------------
  # Nginx global config
  # ---------------------------------------------------------------------------

  services.nginx = {
    enable                   = true;
    recommendedProxySettings = true;
    recommendedTlsSettings   = true;
    recommendedGzipSettings  = true;
    appendHttpConfig = ''
      limit_req_zone $binary_remote_addr zone=emby_auth:10m rate=5r/s;
      ${securityHeaders}
    '';

    # -------------------------------------------------------------------------
    # Emby (intrentaka.com)
    # -------------------------------------------------------------------------

    virtualHosts."intrentaka.com" = {
      serverAliases = [ "www.intrentaka.com" ];
      forceSSL   = true;
      enableACME = true;

      locations."^~ /swagger" = {
        extraConfig = "return 404;";
      };

      locations."~ ^/(emby/)?Users/AuthenticateByName$" = {
        proxyPass       = "http://${config.homeserver.localhost}:${toString config.homeserver.ports.emby}";
        proxyWebsockets = true;
        extraConfig = ''
          limit_req zone=emby_auth burst=10 nodelay;
          limit_req_status 429;
          ${securityHeaders}
          ${corsHeaders}
        '';
      };

      locations."/" = {
        proxyPass       = "http://${config.homeserver.localhost}:${toString config.homeserver.ports.emby}";
        proxyWebsockets = true;
        extraConfig = ''
          ${securityHeaders}
          ${corsHeaders}
        '';
      };
    };
  };

  # ---------------------------------------------------------------------------
  # Firewall
  # ---------------------------------------------------------------------------

  networking.firewall.allowedTCPPorts = [ 80 443 ];
}
