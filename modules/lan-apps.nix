{ config, pkgs, lib, ... }:

let
  pythonEnv = pkgs.python3.withPackages (ps: with ps; [ flask waitress requests mutagen ]);
  nodeEnv = pkgs.nodejs_20;
  npmInstallOnce = pkg: stateDir: "${pkgs.bash}/bin/bash -c 'test -d ${stateDir}/node_modules/${pkg} || ${nodeEnv}/bin/npm install --prefix ${stateDir} ${pkg}'";
in
{
  services.nginx.virtualHosts."homepage-lan" = {
    listen = [{ addr = "0.0.0.0"; port = 5000; }];
    root = ../apps/homepage;
  };

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
        "NODE_PATH=/var/lib/math-tutor/node_modules"
        "HOME=/var/lib/math-tutor"
      ];
      ExecStartPre = npmInstallOnce "express" "/var/lib/math-tutor";
      ExecStart = "${nodeEnv}/bin/node ${../apps/math-tutor}/server.js";
      Restart = "on-failure";
    };
  };

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
        "NODE_PATH=/var/lib/typing-tutor/node_modules"
        "HOME=/var/lib/typing-tutor"
      ];
      ExecStartPre = npmInstallOnce "express" "/var/lib/typing-tutor";
      ExecStart = "${nodeEnv}/bin/node ${../apps/typing-tutor}/server.js";
      Restart = "on-failure";
    };
  };

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
      ];
      EnvironmentFile = "/var/lib/music-info/secrets.env";
      ExecStart = "${pythonEnv}/bin/python3 ${../apps/music-info-project}/app.py";
      Restart = "on-failure";
    };
  };

  systemd.services.music-scanner = {
    description = "Music Library Scanner (manual trigger only)";
    serviceConfig = {
      Type = "oneshot";
      User = "music-info";
      Group = "music-info";
      StateDirectory = "music-info";
      Environment = [ "MUSIC_DB_PATH=/var/lib/music-info/music.db" "MUSIC_PATH=/mnt/server-pc/Media/Albums" ];
      EnvironmentFile = "/var/lib/music-info/secrets.env";
      WorkingDirectory = "${../apps/music-info-project}";
      ExecStart = "${pythonEnv}/bin/python3 -u ${../apps/music-info-project}/music_scanner.py";
    };
  };

  networking.firewall.allowedTCPPorts = [ 3000 3001 5000 5001 5010 ];
}
