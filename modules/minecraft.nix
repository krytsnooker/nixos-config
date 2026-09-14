{ config, pkgs, lib, inputs, ... }:

let
  mcServerName = "survival";
  mcUnit       = "minecraft-server-${mcServerName}.service";
  mcDataDir    = "/var/lib/minecraft/${mcServerName}";
  propsFile    = "${mcDataDir}/server.properties";
  pythonEnv    = pkgs.python3.withPackages (ps: with ps; [ flask waitress ]);

  backupScript = pkgs.writeShellScript "mc-backup" ''
    set -euo pipefail

    src="/var/lib/minecraft/${mcServerName}/"
    dst="/mnt/server-pc/Backups/minecraft"

    # Skip silently if the DAS isn't mounted
    if ! ${pkgs.util-linux}/bin/mountpoint -q /mnt/server-pc; then
      echo "mc-backup: /mnt/server-pc not mounted, skipping" >&2
      exit 0
    fi

    today=$(${pkgs.coreutils}/bin/date +%Y-%m-%d)
    ${pkgs.coreutils}/bin/mkdir -p "$dst/$today"
    ${pkgs.rsync}/bin/rsync -a --delete "$src" "$dst/$today/"

    # Retain the 7 most recent daily backups; remove the rest
    ${pkgs.findutils}/bin/find "$dst" -maxdepth 1 -type d -name '????-??-??' \
      | ${pkgs.coreutils}/bin/sort \
      | ${pkgs.coreutils}/bin/head -n -7 \
      | ${pkgs.findutils}/bin/xargs -r ${pkgs.coreutils}/bin/rm -rf

    echo "mc-backup: done ($today)"
  '';
in
{
  nixpkgs.overlays = [ inputs.nix-minecraft.overlays.default ];

  services.minecraft-servers = {
    enable = true;
    eula = true;
    dataDir = "/var/lib/minecraft";

    servers.${mcServerName} = {
      enable = true;
      autoStart = false;
      package = pkgs.paperServers.paper-1_19_4;
      jvmOpts = "-Xms1G -Xmx3G";
      serverProperties = {
        # Infrastructure settings only — pinned here so nixos-rebuild can't
        # accidentally flip them. User-managed settings live in
        # /var/lib/mc-control/settings.json and are applied by mc-control
        # on every start.
        online-mode = false;
        server-port = 25565;
      };
    };
  };

  # --- mc-control: web UI for starting/stopping and configuring the server ---
  users.groups.mcctl = { };
  users.groups.mcplugins = { };
  users.users.mcctl = {
    isSystemUser = true;
    group = "mcctl";
    extraGroups = [ "mcplugins" "minecraft" ];
  };

  systemd.tmpfiles.rules = [
    "d /var/lib/minecraft/${mcServerName}/plugins 0775 minecraft mcplugins -"
  ];

  security.sudo.extraRules = [
    {
      users = [ "mcctl" ];
      commands = [
        { command = "/run/current-system/sw/bin/systemctl start ${mcUnit}";     options = [ "NOPASSWD" ]; }
        { command = "/run/current-system/sw/bin/systemctl stop ${mcUnit}";      options = [ "NOPASSWD" ]; }
        { command = "/run/current-system/sw/bin/systemctl is-active ${mcUnit}"; options = [ "NOPASSWD" ]; }
        { command = "/run/current-system/sw/bin/cat ${propsFile}";              options = [ "NOPASSWD" ]; }
        { command = "/run/current-system/sw/bin/tee ${propsFile}";              options = [ "NOPASSWD" ]; }
      ];
    }
  ];

  systemd.services.mc-control = {
    description = "Minecraft management web UI";
    after    = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      User  = "mcctl";
      Group = "mcctl";
      StateDirectory = "mc-control";
      Environment = [
        "MC_UNIT=${mcUnit}"
        "MC_DATA_DIR=${mcDataDir}"
        "MC_STATE_DIR=/var/lib/mc-control"
        "MC_CONTROL_PORT=5020"
      ];
      ExecStart = "${pythonEnv}/bin/python3 ${../apps/mc-control}/app.py";
      Restart = "on-failure";
    };
  };

  # --- mc-backup: daily world backup to the DAS ---
  systemd.services.mc-backup = {
    description = "Minecraft world backup to DAS";
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${backupScript}";
    };
  };

  systemd.timers.mc-backup = {
    description = "Daily Minecraft world backup";
    wantedBy = [ "timers.target" ];
    timerConfig = {
      OnCalendar = "*-*-* 03:00:00";
      # If the machine was off at 3 AM, run the backup on next boot
      Persistent = true;
    };
  };

  networking.firewall.allowedTCPPorts = [ 25565 5020 ];
}
