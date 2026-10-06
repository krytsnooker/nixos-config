{ config, pkgs, lib, ... }:

{
  services.vaultwarden = {
    enable = true;
    dbBackend = "sqlite";
    backupDir = "/var/backup/vaultwarden";
    environmentFile = "/var/lib/vaultwarden/secrets.env";
    webVaultPackage = pkgs.vaultwarden-webvault;
    config = {
      DOMAIN = "https://vault.intrentaka.com";
      SIGNUPS_ALLOWED = false;
      ROCKET_ADDRESS = "127.0.0.1";
      ROCKET_PORT = 8222;
      SHOW_PASSWORD_HINT = false;
    };
  };

  services.nginx.virtualHosts."vault.intrentaka.com" = {
    forceSSL = true;
    enableACME = true;
    locations."/" = {
      proxyPass = "http://127.0.0.1:8222";
      proxyWebsockets = true;
      extraConfig = ''
        add_header Strict-Transport-Security "max-age=15552000; includeSubDomains" always;
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header Referrer-Policy "strict-origin-when-cross-origin" always;
        add_header Permissions-Policy "geolocation=(), microphone=(), camera=()" always;
      '';
    };
  };
}
