{ pkgs, ... }:

let
  mjhProxy = pkgs.buildGoModule {
    pname = "mjh-proxy";
    version = "0.1.4";
    src = ../apps/mjh-proxy;
    vendorHash = null;
  };
in
{
  systemd.services.mjh-proxy = {
    description = "MJH IPTV local HLS buffer with ffmpeg";
    after = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      DynamicUser = true;
      RuntimeDirectory = "mjh-proxy";
      ExecStart = "${mjhProxy}/bin/mjh-proxy";
      Environment = [
        "FFMPEG_PATH=${pkgs.ffmpeg}/bin/ffmpeg"
        "RUNTIME_DIR=/run/mjh-proxy"
      ];
      Restart = "on-failure";
      RestartSec = "5s";
    };
  };

  networking.firewall.allowedTCPPorts = [ 5030 ];
}
