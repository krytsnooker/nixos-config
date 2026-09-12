{ config, pkgs, lib, ... }:

let
  pythonEnv = pkgs.python3.withPackages (ps: with ps; [ flask waitress requests mutagen ]);
  nodeEnv = pkgs.nodejs_20;
in
{
  # --- homepage: pure static HTML, just serve it via nginx ---
  services.nginx.virtualHosts."homepage-lan" = {
    listen = [{ addr = "0.0.0.0"; port = 5000; }];
    root = ../apps/homepage;
  };

  # --- lan_pastebin (Flask) ---
  systemd.services.lan-pastebin = {
    description = "LAN Pastebin";
    after = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      DynamicUser = true;
      StateDirectory = "lan-pastebin";
      WorkingDirectory = "${../apps/lan_pastebin}";
      Environment = [
        "PASTEBIN_DB_PATH=/var/lib/lan-pastebin/pastes.db"
        "PASTEBIN_PORT=5001"
      ];
      ExecStart = "${pythonEnv}/bin/python3 ${../apps/lan_pastebin}/app.py";
      Restart = "on-failure";
    };
  };

  # --- math-tutor (Node/Express) ---
  systemd.services.math-tutor = {
    description = "Math Tutor";
    after = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      DynamicUser = true;
      StateDirectory = "math-tutor";
      Environment = [
        "MATH_TUTOR_PORT=3001"
        "MATH_TUTOR_LOGS_FILE=/var/lib/math-tutor/logs.json"
      ];
      ExecStart = "${nodeEnv}/bin/node ${../apps/math-tutor}/server.js";
      Restart = "on-failure";
    };
  };

  # --- typing-tutor (Node/Express) ---
  systemd.services.typing-tutor = {
    description = "Typing Tutor";
    after = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      DynamicUser = true;
      StateDirectory = "typing-tutor";
      Environment = [
        "TYPING_TUTOR_PORT=3000"
        "TYPING_TUTOR_LOGS_FILE=/var/lib/typing-tutor/logs.json"
      ];
      ExecStart = "${nodeEnv}/bin/node ${../apps/typing-tutor}/server.js";
      Restart = "on-failure";
    };
  };

  # --- music-info-project (Flask) ---
  # NOT DynamicUser: the scanner (music-scanner.service, run manually) and
  # this web service both need to read/write the SAME music.db in a shared
  # StateDirectory, and both need "sambashare" group membership to reach the
  # CIFS-mounted DAS. Two DynamicUser services can't safely share a
  # StateDirectory (systemd re-chowns it to whichever one started most
  # recently), so a fixed system user is used instead.
  users.groups.music-info = { };
  users.users.music-info = {
    isSystemUser = true;
    group = "music-info";
    extraGroups = [ "sambashare" ];
  };

  systemd.services.music-info = {
    description = "Music Info Database";
    after = [ "network.target" "mnt-server\\x2dpc.mount" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      User = "music-info";
      Group = "music-info";
      StateDirectory = "music-info";
      Environment = [
        "MUSIC_DB_PATH=/var/lib/music-info/music.db"
        "MUSIC_INFO_PORT=5010"
        # DISCOGS_TOKEN / EMBY_API_KEY / EMBY_BASE_URL / EMBY_EXTERNAL_URL /
        # MUSIC_PATH come from the EnvironmentFile below, kept outside the
        # Nix store since they're secrets.
      ];
      EnvironmentFile = "/var/lib/music-info/secrets.env"; # TODO: create this file (see README), chmod 600
      ExecStart = "${pythonEnv}/bin/python3 ${../apps/music-info-project}/app.py";
      Restart = "on-failure";
    };
  };

  # Manual rescan: `systemctl start music-scanner.service` after adding new
  # music to the DAS. Not on a timer — trigger it yourself when needed.
  systemd.services.music-scanner = {
    description = "Music Library Scanner (manual trigger only)";
    serviceConfig = {
      Type = "oneshot";
      User = "music-info";
      Group = "music-info";
      StateDirectory = "music-info";
      Environment = [ "MUSIC_DB_PATH=/var/lib/music-info/music.db" ];
      EnvironmentFile = "/var/lib/music-info/secrets.env";
      WorkingDirectory = "${../apps/music-info-project}";
      ExecStart = "${pythonEnv}/bin/python3 ${../apps/music-info-project}/music_scanner.py";
    };
  };

  networking.firewall.allowedTCPPorts = [ 3000 3001 5000 5001 5010 ];
}
