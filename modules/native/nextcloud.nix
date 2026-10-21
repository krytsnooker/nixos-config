{ config, pkgs, lib, ... }:

{
  options.homeserver.nextcloudUid = lib.mkOption {
    type = lib.types.int;
    default = 991;
    description = "UID for the nextcloud service user; must match the uid= option on the CIFS data mount in configuration.nix.";
  };

  config = let
    dataDir    = "${config.services.nextcloud.home}/data";
    secretFile = config.services.nextcloud.secretFile;
    occ        = "${config.services.nextcloud.occ}/bin/nextcloud-occ";
  in {

    # ---------------------------------------------------------------------------
    # Core Nextcloud configuration
    # ---------------------------------------------------------------------------

    services.nextcloud = {
      enable                = true;
      package               = pkgs.nextcloud34;
      hostName              = "cloud.intrentaka.com";
      https                 = true;
      database.createLocally = true;
      configureRedis        = true;
      secretFile            = "/var/lib/nextcloud/nc-secrets.php";

      config = {
        dbtype       = "mysql";
        adminuser    = "admin";
        adminpassFile = "/var/lib/nextcloud-admin-pass";
      };

      settings = {
        trusted_proxies          = [ "127.0.0.1" ];
        "overwrite.cli.url"      = "https://cloud.intrentaka.com";
        overwriteprotocol        = "https";
        maintenance_window_start = 1;
        mail_from_address        = "krytsnooker";
        mail_smtpmode            = "smtp";
        mail_domain              = "gmail.com";
        mail_smtphost            = "smtp.gmail.com";
        mail_smtpport            = 587;
        mail_smtpauth            = true;
        mail_smtpname            = "krytsnooker@gmail.com";
      };
    };

    # UID declared once here; configuration.nix reads it for the CIFS mount
    # uid= option so the two can never drift.
    users.users.nextcloud.uid         = config.homeserver.nextcloudUid;
    users.users.nextcloud.extraGroups = [ "sambashare" ];

    # Nextcloud services must wait for the data CIFS mount before starting.
    systemd.services.nextcloud-setup = {
      after    = [ "var-lib-nextcloud-data.mount" ];
      requires = [ "var-lib-nextcloud-data.mount" ];
    };

    services.nginx.virtualHosts."cloud.intrentaka.com" = {
      forceSSL   = true;
      enableACME = true;
      locations."= /status.php" = {
        extraConfig = "return 404;";
      };
    };


    # ---------------------------------------------------------------------------
    # Security hardening
    # ---------------------------------------------------------------------------

    # Disable shell-execution PHP functions. Preview generation that previously
    # used exec/proc_open for ImageMagick/FFmpeg now runs via the
    # nextcloud-preview-gen systemd timer instead of inline during web requests.
    services.nextcloud.phpOptions = {
      "disable_functions" = "system,passthru,pcntl_exec,exec,proc_open";
    };

    # Sandbox the PHP-FPM worker process.
    systemd.services.phpfpm-nextcloud = {
      after    = [ "var-lib-nextcloud-data.mount" ];
      requires = [ "var-lib-nextcloud-data.mount" ];
      serviceConfig = {
        NoNewPrivileges       = true;
        PrivateTmp            = true;
        ProtectKernelTunables = true;
        ProtectKernelModules  = true;
        ProtectControlGroups  = true;
        RestrictSUIDSGID      = true;
        LockPersonality       = true;
      };
    };

    # TEMPORARY BLOCK — CVE-2026-109618/109619 PHP RCE.
    # Remove extraConfig once nixpkgs is updated to a patched PHP version.
    services.nginx.virtualHosts."cloud.intrentaka.com".extraConfig = "return 503;";


    # ---------------------------------------------------------------------------
    # Preview generation timer
    # Runs outside PHP-FPM so exec/proc_open can be disabled in the web process.
    # Walks user file trees every 15 minutes and generates missing previews.
    # ---------------------------------------------------------------------------

    systemd.services.nextcloud-preview-gen = let
      previewScript = pkgs.writeShellScript "nextcloud-preview-gen" ''
        find ${dataDir} -path "*/files/*" -type f \
          \( -name "*.jpg" -o -name "*.jpeg" -o -name "*.png" -o -name "*.gif" \
             -o -name "*.bmp" -o -name "*.tiff" -o -name "*.webp" \
             -o -name "*.pdf" \
             -o -name "*.mp4" -o -name "*.mkv" -o -name "*.mov" -o -name "*.avi" \) \
        | while IFS= read -r f; do
            ${occ} preview:generate "''${f#${dataDir}}" --quiet || true
          done
      '';
    in {
      description = "Generate Nextcloud previews";
      after    = [ "phpfpm-nextcloud.service" "var-lib-nextcloud-data.mount" ];
      requires = [ "var-lib-nextcloud-data.mount" ];
      serviceConfig = {
        Type            = "oneshot";
        User            = "nextcloud";
        ExecStart       = "${previewScript}";
        LoadCredential  = "secret_file:${secretFile}";
      };
    };

    systemd.timers.nextcloud-preview-gen = {
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnCalendar = "*:0/15";
        Persistent = true;
      };
    };

  };
}
