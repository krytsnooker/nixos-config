{ pkgs, ... }:

let
  mjhProxy = pkgs.buildGoModule {
    pname = "mjh-proxy";
    version = "0.1.0";
    src = ../apps/mjh-proxy;
    vendorHash = null;
  };
in
{
  systemd.services.mjh-proxy = {
    description = "MJH IPTV stream proxy with DAI session refresh";
    after = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      DynamicUser = true;
      ExecStart = "${mjhProxy}/bin/mjh-proxy";
      Restart = "on-failure";
      RestartSec = "5s";
    };
  };

  networking.firewall.allowedTCPPorts = [ 5030 ];
}
