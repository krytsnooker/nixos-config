{ config, pkgs, lib, ... }:

{
  # mariadb.service equivalent
  services.mysql = {
    enable = true;
    package = pkgs.mariadb;
  };

  # postgresql@16-main.service equivalent
  services.postgresql = {
    enable = true;
    package = pkgs.postgresql_16;
  };

  # Nextcloud (see nextcloud.nix) needs its DB reachable locally; both
  # services default to listening on localhost only via their unix sockets,
  # which is fine for same-host apps like Nextcloud/Emby-adjacent tooling.
}
