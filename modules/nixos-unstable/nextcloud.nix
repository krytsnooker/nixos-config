{ config, pkgs, pkgsUnstable, lib, ... }:

{
  config = let
    dataDir    = "${config.services.nextcloud.home}/data";
    secretFile = config.services.nextcloud.secretFile;
    occ        = "${config.services.nextcloud.occ}/bin/nextcloud-occ";
  in {

    # ---------------------------------------------------------------------------
    # Core Nextcloud configuration
    # ---------------------------------------------------------------------------

    services.nextcloud = {
      enable                 = true;
      package                = pkgsUnstable.nextcloud34;
      hostName               = "cloud.intrentaka.com";
      https                  = true;
      database.createLocally = true;
      configureRedis         = true;
      secretFile             = "/var/lib/nextcloud/nc-secrets.php";

      config = {
        dbtype        = "mysql";
        adminuser     = "admin";
        adminpassFile = "/var/lib/nextcloud-admin-pass";
      };

      settings = {
        trusted_proxies          = [ config.homeserver.localhost ];
        "overwrite.cli.url"      = "https://${config.services.nextcloud.hostName}";
        overwriteprotocol        = "https";
        maintenance_window_start = 1;
        mail_from_address        = "krytsnooker";
        mail_smtpmode            = "smtp";
        mail_domain              = "gmail.com";
        mail_smtphost            = "smtp.gmail.com";
        mail_smtpport            = 587;
        mail_smtpauth            = true;
        mail_smtpname            = "krytsnooker@gmail.com";
        preview_max_x            = 256;
        preview_max_y            = 256;
      };
    };

    # ---------------------------------------------------------------------------
    # Nginx TLS/ACME
    # ---------------------------------------------------------------------------
    # services.nextcloud's https=true only affects URL generation inside the
    # app; the actual nginx vhost's TLS/ACME must be declared explicitly, or
    # nginx serves this vhost with no real certificate at all.

    services.nginx.virtualHosts.${config.services.nextcloud.hostName} = {
      forceSSL   = true;
      enableACME = true;
    };

    # ---------------------------------------------------------------------------
    # Users
    # ---------------------------------------------------------------------------

    users.users.nextcloud = {
      uid         = config.homeserver.uids.unstable.nextcloud;
      extraGroups = [ "sambashare" ];
    };

    # ---------------------------------------------------------------------------
    # CIFS dependency
    # ---------------------------------------------------------------------------

    systemd.services.nextcloud-setup = {
      after         = [ "var-lib-nextcloud-data.mount" ];
      requires      = [ "var-lib-nextcloud-data.mount" ];
      serviceConfig = config.homeserver.sandbox;
    };

    systemd.services.nextcloud-update-db.serviceConfig = config.homeserver.sandbox;
    systemd.services.nextcloud-cron.serviceConfig      = config.homeserver.sandbox;

    # ---------------------------------------------------------------------------
    # Security hardening
    # ---------------------------------------------------------------------------

    services.nextcloud.phpOptions = {
      "disable_functions" = "system,passthru,pcntl_exec,exec,proc_open";
    };

    systemd.services.phpfpm-nextcloud = {
      after    = [ "var-lib-nextcloud-data.mount" ];
      requires = [ "var-lib-nextcloud-data.mount" ];
      serviceConfig = config.homeserver.sandbox;
    };

    # ---------------------------------------------------------------------------
    # Preview generation timer
    # ---------------------------------------------------------------------------

    systemd.services.nextcloud-preview-gen = let
      previewScript = pkgs.writeShellScript "nextcloud-preview-gen" ''
        extensions=(
          jpg jpeg png gif
          bmp tiff webp
          pdf
          mp4 mkv mov avi
        )

        name_args=()
        for ext in "''${extensions[@]}"; do
          name_args+=(-o -name "*.$ext")
        done

        find ${dataDir} -path "*/files/*" -type f \( ''${name_args[@]:1} \) \
          | while IFS= read -r f; do
              ${occ} preview:generate "''${f#${dataDir}}" --quiet || true
            done
      '';
    in {
      description = "Generate Nextcloud previews";
      after    = [ "phpfpm-nextcloud.service" "var-lib-nextcloud-data.mount" ];
      requires = [ "var-lib-nextcloud-data.mount" ];
      serviceConfig = config.homeserver.sandbox // {
        Type           = "oneshot";
        User           = "nextcloud";
        ExecStart      = previewScript;
        LoadCredential = "secret_file:${secretFile}";
      };
    };

    systemd.timers.nextcloud-preview-gen = {
      enable      = false;
      wantedBy    = [ "timers.target" ];
      timerConfig = {
        OnCalendar = "*:0/15";
        Persistent = true;
      };
    };

  };
}
