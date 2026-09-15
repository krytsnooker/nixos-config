{ config, pkgs, lib, unstablePkgs, ... }:

{
  options.homeserver.nextcloudUid = lib.mkOption {
    type = lib.types.int;
    default = 991;
    description = "UID for the nextcloud service user; must match the uid= option on the CIFS data mount in hosts/homeserver/configuration.nix.";
  };

  config = {
    services.nextcloud = {
      enable = true;
      package = unstablePkgs.nextcloud34;
      hostName = "cloud.intrentaka.com";
      https = true;
      database.createLocally = true;
      configureRedis = true;
      secretFile = "/var/lib/nextcloud/nc-secrets.php";
      config = {
        dbtype = "mysql";
        adminuser = "admin";
        adminpassFile = "/var/lib/nextcloud-admin-pass";
      };
      settings = {
        trusted_proxies = [ "127.0.0.1" ];
        "overwrite.cli.url" = "https://cloud.intrentaka.com";
        overwriteprotocol = "https";
        maintenance_window_start = 1;
        mail_from_address = "krytsnooker";
        mail_smtpmode = "smtp";
        mail_domain = "gmail.com";
        mail_smtphost = "smtp.gmail.com";
        mail_smtpport = 587;
        mail_smtpauth = true;
        mail_smtpname = "krytsnooker@gmail.com";
      };
    };

    # UID is defined once as homeserver.nextcloudUid above; configuration.nix
    # reads it for the CIFS mount uid= option so the two can never drift.
    users.users.nextcloud.uid = config.homeserver.nextcloudUid;
    users.users.nextcloud.extraGroups = [ "sambashare" ];

    # Nextcloud services must wait for the data CIFS mount before starting
    systemd.services.nextcloud-setup = {
      after = [ "var-lib-nextcloud-data.mount" ];
      requires = [ "var-lib-nextcloud-data.mount" ];
    };
    systemd.services.phpfpm-nextcloud = {
      after = [ "var-lib-nextcloud-data.mount" ];
      requires = [ "var-lib-nextcloud-data.mount" ];
    };

    services.nginx.virtualHosts."cloud.intrentaka.com" = {
      forceSSL = true;
      enableACME = true;
      locations."= /status.php" = {
        extraConfig = "return 404;";
      };
    };
  };
}
