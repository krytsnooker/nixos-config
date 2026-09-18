{ config, pkgs, lib, inputs, ... }:

let
  mcServerName    = "survival";
  mcUnit          = "minecraft-server-${mcServerName}.service";
  mcDataDir       = "/var/lib/minecraft/${mcServerName}";
  mcStateDir      = "/var/lib/mc-control";
  propsFile       = "${mcDataDir}/server.properties";
  pythonEnv       = pkgs.python3.withPackages (ps: with ps; [ flask waitress ]);

  neoforgeVersion = "1.20.1-47.1.106";

  # Applies mc-control settings.json onto server.properties. Called from the
  # ExecStart wrapper so it runs after nix-minecraft's ExecStartPre has already
  # regenerated server.properties from the Nix store.
  applySettingsScript = pkgs.writeScript "mc-apply-settings" ''
    #!${pkgs.python3}/bin/python3
    import json, os, sys

    SETTINGS = "${mcStateDir}/settings.json"
    PROPS    = "${propsFile}"
    READONLY = {"online-mode", "server-port"}

    if not os.path.exists(SETTINGS):
        sys.exit(0)

    with open(SETTINGS) as f:
        try:
            settings = json.load(f)
        except json.JSONDecodeError:
            sys.exit(0)

    try:
        with open(PROPS) as f:
            lines = f.readlines()
    except FileNotFoundError:
        sys.exit(0)

    applied = set()
    out = []
    for line in lines:
        s = line.rstrip()
        if s and not s.startswith("#"):
            key = s.split("=")[0].strip()
            if key in settings and key not in READONLY:
                out.append(f"{key}={settings[key]}\n")
                applied.add(key)
                continue
        out.append(line if line.endswith("\n") else line + "\n")

    for key, val in settings.items():
        if key not in applied and key not in READONLY:
            out.append(f"{key}={val}\n")

    with open(PROPS, "w") as f:
        f.writelines(out)
  '';

  # Wrapper that nix-minecraft calls as: minecraft-server -Xms1G -Xmx3G
  # Writes JVM args to user_jvm_args.txt (NeoForge's args file) then runs run.sh
  neoforge1201Package = pkgs.writeShellScriptBin "minecraft-server" ''
    export PATH="${pkgs.jdk21}/bin:$PATH"
    printf '%s\n' "$@" > user_jvm_args.txt
    ${applySettingsScript} || true
    exec ${pkgs.bash}/bin/bash ./run.sh
  '';

  # One-time installer: downloads and runs the NeoForge 1.20.1 installer into the data dir
  neoforge1201SetupScript = pkgs.writeShellScript "neoforge-1201-setup" ''
    set -euo pipefail
    MARKER="${mcDataDir}/.neoforge-${neoforgeVersion}-installed"
    [ -f "$MARKER" ] && exit 0

    echo "Downloading NeoForge ${neoforgeVersion} installer..."
    TMPDIR=$(${pkgs.coreutils}/bin/mktemp -d)
    trap '${pkgs.coreutils}/bin/rm -rf "$TMPDIR"' EXIT

    ${pkgs.curl}/bin/curl -fL \
      "https://maven.neoforged.net/releases/net/neoforged/forge/${neoforgeVersion}/forge-${neoforgeVersion}-installer.jar" \
      -o "$TMPDIR/installer.jar"

    echo "Installing NeoForge server to ${mcDataDir}..."
    cd "${mcDataDir}"
    ${pkgs.jdk21}/bin/java -jar "$TMPDIR/installer.jar" --installServer "${mcDataDir}"

    chmod +x "${mcDataDir}/run.sh"
    touch "$MARKER"
    echo "NeoForge ${neoforgeVersion} installation complete"
  '';

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
    ${pkgs.rsync}/bin/rsync -a --copy-links --delete "$src" "$dst/$today/"

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
      package = neoforge1201Package;
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
  users.groups.mcmods = { };
  users.users.mcctl = {
    isSystemUser = true;
    group = "mcctl";
    extraGroups = [ "mcmods" "minecraft" ];
  };

  systemd.tmpfiles.rules = [
    "d /var/lib/minecraft/${mcServerName}/mods 0775 minecraft mcmods -"
  ];

  # Runs once to download and install NeoForge 1.20.1 into the data dir
  systemd.services.neoforge-1201-setup = {
    description = "Install NeoForge ${neoforgeVersion} server";
    wantedBy = [ mcUnit ];
    before   = [ mcUnit ];
    serviceConfig = {
      Type             = "oneshot";
      RemainAfterExit  = true;
      User             = "minecraft";
      WorkingDirectory = mcDataDir;
      ExecStart        = neoforge1201SetupScript;
    };
  };

  # Ensure the server service waits for the installer to finish
  systemd.services."minecraft-server-${mcServerName}" = {
    after    = [ "neoforge-1201-setup.service" ];
    requires = [ "neoforge-1201-setup.service" ];
  };

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
