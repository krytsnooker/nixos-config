{ config, pkgs, lib, ... }:

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
      package                = pkgs.nextcloud34;
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
        trusted_proxies          = [ "127.0.0.1" ];
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
      after    = [ "var-lib-nextcloud-data.mount" ];
      requires = [ "var-lib-nextcloud-data.mount" ];
    };

    # ---------------------------------------------------------------------------
    # Nginx
    # ---------------------------------------------------------------------------

    # TEMPORARY — CVE-2026-109618/109619 PHP RCE; remove extraConfig once nixpkgs has patched PHP.
    services.nginx.virtualHosts.${config.services.nextcloud.hostName} = {
      forceSSL    = true;
      enableACME  = true;
      extraConfig = "return 503;";
      locations."= /status.php" = {
        extraConfig = "return 404;";
      };
    };

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
      serviceConfig = {
        Type           = "oneshot";
        User           = "nextcloud";
        ExecStart      = previewScript;
        LoadCredential = "secret_file:${secretFile}";
      };
    };

    systemd.timers.nextcloud-preview-gen = {
      wantedBy    = [ "timers.target" ];
      timerConfig = {
        OnCalendar = "*:0/15";
        Persistent = true;
      };
    };

  };
}
