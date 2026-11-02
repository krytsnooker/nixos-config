{ config, pkgs, lib, ... }:

{
  # ---------------------------------------------------------------------------
  # Host Filepaths — browse host directories; qBittorrent category-path bridge
  # ---------------------------------------------------------------------------

  users.groups.host-filepaths = { gid = config.homeserver.gids.hostFilepaths; };
  users.users.host-filepaths = {
    isSystemUser = true;
    uid   = config.homeserver.uids.services.hostFilepaths;
    group = "host-filepaths";
    extraGroups = [ "sambashare" ];
  };

  systemd.services.host-filepaths = {
    description = "Host Filepaths";
    after = [ "network.target" "mnt-server\\x2dpc.mount" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = let
      pkg = pkgs.buildGoModule {
        pname = "host-filepaths";
        version = "0.1.0";
        src = ../../apps/host-filepaths;
        subPackages = [ "cmd/server" ];
        vendorHash = null;
      };
    in {
      User   = "host-filepaths";
      Group  = "host-filepaths";
      StateDirectory = "host-filepaths";
      EnvironmentFile = "-/var/lib/host-filepaths/secrets.env";
      ExecStart = "${pkg}/bin/server -addr 0.0.0.0:${toString config.homeserver.ports.hostFilepaths} -media ${config.homeserver.paths.media} -web ${../../apps/host-filepaths/web} -qbittorrent-url http://192.168.15.1:${toString config.homeserver.ports.qbittorrent}";
      Restart = "on-failure";
    } // config.homeserver.sandbox;
  };

  networking.firewall.allowedTCPPorts = [ config.homeserver.ports.hostFilepaths ];
}
