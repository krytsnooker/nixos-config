{ config, pkgs, pkgsUnstable, lib, ... }:

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
    package         = pkgsUnstable.vaultwarden;
    dbBackend       = "sqlite";
    backupDir       = "/var/backup/vaultwarden";
    environmentFile = "/var/lib/vaultwarden/secrets.env";
    webVaultPackage = pkgsUnstable.vaultwarden.webvault;
    config = {
      DOMAIN             = "https://${domain}";
      SIGNUPS_ALLOWED    = false;
      ROCKET_ADDRESS     = config.homeserver.localhost;
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
      proxyPass       = "http://${config.homeserver.localhost}:${toString rocketPort}";
      proxyWebsockets = true;
    };
  };
}
