{ config, pkgs, lib, inputs, ... }:

let
  # Single source of truth for the unit name — shared between the server
  # definition below, the sudo rule, and mc-control's environment, so they
  # can't drift out of sync with each other.
  mcServerName = "survival";
  mcUnit = "minecraft-server-${mcServerName}.service";
  pythonEnv = pkgs.python3.withPackages (ps: with ps; [ flask waitress ]);
in
{
  nixpkgs.overlays = [ inputs.nix-minecraft.overlays.default ];

  # NOTE: nix-minecraft is a community flake (Infinidoge/nix-minecraft), not
  # mainline nixpkgs — it's what makes an old pinned version like 1.19.4
  # available at all (mainline nixpkgs only ships whatever single version is
  # current). Verify the exact package attribute name
  # (pkgs.paperServers.paper-1_19_4) and the "autoStart" option still match
  # on the workstation before relying on this — community flake APIs shift
  # more often than mainline nixpkgs.
  services.minecraft-servers = {
    enable = true;
    eula = true;
    dataDir = "/var/lib/minecraft";

    servers.${mcServerName} = {
      enable = true;
      autoStart = false; # on-demand only, triggered via the homepage button (mc-control)
      package = pkgs.paperServers.paper-1_19_4;
      jvmOpts = "-Xms1G -Xmx3G"; # capped — see the RAM budget discussion, don't let this crowd out Emby/Nextcloud
      serverProperties = {
        online-mode = false; # required for offline/cracked-style clients (e.g. via PollyMC) — no Mojang account check
        server-port = 25565;
        white-list = false;
        motd = "Home LAN Server";
      };
    };
  };

  # --- mc-control: lets the homepage start/stop the server on demand ---
  # Fixed user (not DynamicUser) because the sudo rule below needs a stable
  # username to grant permission to.
  users.groups.mcctl = { };
  users.users.mcctl = {
    isSystemUser = true;
    group = "mcctl";
  };

  # The ONLY elevated thing mcctl can do: these three exact commands on this
  # one unit. Not a general systemctl grant, not root, nothing else.
  security.sudo.extraRules = [
    {
      users = [ "mcctl" ];
      commands = [
        { command = "/run/current-system/sw/bin/systemctl start ${mcUnit}"; options = [ "NOPASSWD" ]; }
        { command = "/run/current-system/sw/bin/systemctl stop ${mcUnit}"; options = [ "NOPASSWD" ]; }
        { command = "/run/current-system/sw/bin/systemctl is-active ${mcUnit}"; options = [ "NOPASSWD" ]; }
      ];
    }
  ];

  systemd.services.mc-control = {
    description = "Minecraft start/stop API for the LAN homepage";
    after = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      User = "mcctl";
      Group = "mcctl";
      Environment = [
        "MC_UNIT=${mcUnit}"
        "MC_CONTROL_PORT=5020"
      ];
      ExecStart = "${pythonEnv}/bin/python3 ${../apps/mc-control}/app.py";
      Restart = "on-failure";
    };
  };

  # 25565: Minecraft. LAN-only in practice because this port is never in the
  # nginx/ACME reverse-proxy list and was never forwarded on the router —
  # this firewall rule just allows LAN traffic to reach it locally, it
  # doesn't by itself expose it to the internet (that boundary is the
  # router's port-forward table, which this config can't see or control).
  # 5020: mc-control API.
  networking.firewall.allowedTCPPorts = [ 25565 5020 ];
}
