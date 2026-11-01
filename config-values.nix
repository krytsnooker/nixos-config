{ lib, ... }:

{
  options.homeserver = {

    # -------------------------------------------------------------------------
    # UIDs
    # -------------------------------------------------------------------------

    uids = {

      podman = lib.mapAttrs (name: default:
        lib.mkOption { type = lib.types.int; inherit default; }
      ) {
        emby           = 980;
        audiobookshelf = 979;
      };

      unstable = lib.mapAttrs (name: default:
        lib.mkOption { type = lib.types.int; inherit default; }
      ) {
        nextcloud = 991;
      };

      services = lib.mapAttrs (name: default:
        lib.mkOption { type = lib.types.int; inherit default; }
      ) {
        musicInfo   = 992;
        romTransfer = 985;
        mcctl       = 994;
      };

    };

    # -------------------------------------------------------------------------
    # GIDs
    # -------------------------------------------------------------------------

    gids = lib.mapAttrs (name: default:
      lib.mkOption { type = lib.types.int; inherit default; }
    ) {
      sambashare  = 125;
      musicInfo   = 991;
      romTransfer = 979;
      mcctl       = 993;
      mcmods      = 980;
    };

    # -------------------------------------------------------------------------
    # Shared paths
    # -------------------------------------------------------------------------

    paths = lib.mapAttrs (name: default:
      lib.mkOption { type = lib.types.str; inherit default; }
    ) {
      media   = "/mnt/server-pc/Media";
      backups = "/mnt/server-pc/Backups";
    };

    # -------------------------------------------------------------------------
    # Sandbox
    # -------------------------------------------------------------------------

    sandbox = lib.mkOption {
      type    = lib.types.attrsOf lib.types.bool;
      default = {
        NoNewPrivileges       = true;
        PrivateTmp            = true;
        ProtectKernelTunables = true;
        ProtectKernelModules  = true;
        ProtectControlGroups  = true;
        RestrictSUIDSGID      = true;
        LockPersonality       = true;
        ProtectClock          = true;
        ProtectHostname       = true;
        ProtectKernelLogs     = true;
      };
    };

    # -------------------------------------------------------------------------
    # Addresses
    # -------------------------------------------------------------------------

    localhost = lib.mkOption {
      type    = lib.types.str;
      default = "127.0.0.1";
    };

    ispIp = lib.mkOption {
      type        = lib.types.str;
      default     = "119.17.142.110";
      description = "Admin's home ISP IP; fail2ban allowlist. Update if it changes.";
    };

    lanSubnet = lib.mkOption {
      type    = lib.types.str;
      default = "192.168.0.0/24";
    };

    # -------------------------------------------------------------------------
    # Ports
    # -------------------------------------------------------------------------

    ports = lib.mapAttrs (name: default:
      lib.mkOption { type = lib.types.port; inherit default; }
    ) {
      homepage       = 5000;
      lanPastebin    = 5001;
      typingTutor    = 3000;
      mathTutor      = 3001;
      musicInfo      = 5010;
      romTransfer    = 5035;
      mcControl      = 5020;
      emby           = 8096;
      embyHttps      = 8920;
      audiobookshelf = 13378;
      pihole         = 8083;
      piholeInternal = 8080;
      dns            = 53;
      minecraft      = 25565;
      grafana            = 3010;
      loki               = 3100;
      alloy              = 12345;
      qbittorrent        = 8090;
      qbittorrentTorrent = 6881;
      wireguard          = 51820;
    };

  };
}
