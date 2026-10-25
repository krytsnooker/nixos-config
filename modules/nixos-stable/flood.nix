{ config, pkgs, lib, ... }:

{
  # ---------------------------------------------------------------------------
  # Flood — web UI for qbittorrent with server-side folder browser
  # ---------------------------------------------------------------------------

  services.flood = {
    enable    = true;
    port      = config.homeserver.ports.flood;
    host      = "0.0.0.0";
    extraArgs = [ "--allowedpath" "/mnt/server-pc" ];
  };

  systemd.services.flood.serviceConfig = config.homeserver.sandbox // {
    SupplementaryGroups = [ "sambashare" ];
  };

  networking.firewall.allowedTCPPorts = with config.homeserver.ports; [ flood ];
}
