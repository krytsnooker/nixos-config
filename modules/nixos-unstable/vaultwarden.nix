{ config, pkgs, lib, ... }:

let
  rocketPort = 8222;
  domain     = "vault.intrentaka.com";
in
{
  # ---------------------------------------------------------------------------
  # Vaultwarden
  # ---------------------------------------------------------------------------

  services.vaultwarden = {
    enable          = true;
    dbBackend       = "sqlite";
    backupDir       = "/var/backup/vaultwarden";
    environmentFile = "/var/lib/vaultwarden/secrets.env";
    webVaultPackage = pkgs.vaultwarden-webvault;
    config = {
      DOMAIN             = "https://${domain}";
      SIGNUPS_ALLOWED    = false;
      ROCKET_ADDRESS     = "127.0.0.1";
      ROCKET_PORT        = rocketPort;
      SHOW_PASSWORD_HINT = false;
    };
  };

  # ---------------------------------------------------------------------------
  # Nginx
  # ---------------------------------------------------------------------------

  services.nginx.virtualHosts.${domain} = {
    forceSSL   = true;
    enableACME = true;
    locations."/" = {
      proxyPass       = "http://127.0.0.1:${toString rocketPort}";
      proxyWebsockets = true;
    };
  };
}
