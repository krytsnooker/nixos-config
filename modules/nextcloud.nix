{ config, pkgs, lib, ... }:

{
  # Native replacement for the old snap-based Nextcloud install.
  # Migration path: use `nextcloud-occ maintenance:mode --on`, export the old
  # instance with `occ maintenance:data-fingerprint` / rsync the data dir,
  # then import here — do this as its own step once the base system is
  # validated, not as part of the first boot.
  services.nextcloud = {
    enable = true;
    # CRITICAL: your live instance is Nextcloud 34.0.3. Nextcloud refuses to
    # open a data dir with a NEWER version than the server code — you cannot
    # downgrade. Before attempting the data migration, confirm the pinned
    # nixpkgs revision actually offers a nextcloudNN package >= 34 (check
    # search.nixos.org for the flake's nixpkgs rev, or `nix search
    # nixpkgs nextcloud` on the workstation). If it only offers an older
    # major version, point flake.nix at a newer nixpkgs input instead of
    # trying to force this package version.
    package = pkgs.nextcloud30; # TODO: bump to match/exceed 34.x — see note above
    hostName = "cloud.intrentaka.com";
    https = true; # module wires forceSSL + enableACME on its auto-created vhost
    database.createLocally = true;
    configureRedis = true; # matches the live config's Redis-backed locking/memcache
    config = {
      dbtype = "mysql"; # live config confirmed mysql (mariadb), not postgres
      adminuser = "admin";
      adminpassFile = "/var/lib/nextcloud-admin-pass"; # TODO: create this file, chmod 600, root-owned
      trustedProxies = [ "127.0.0.1" ]; # required since nginx proxies to it locally
    };
    # Point Nextcloud's data directory at the remote DAS mount (see
    # hosts/homeserver/configuration.nix) once it's confirmed working:
    # datadir = "/mnt/server-pc/nextcloud-data";
  };

  # The nextcloud module auto-creates the cloud.intrentaka.com nginx vhost
  # (SSL, cert, proxy-to-php-fpm all handled by the module) once
  # services.nginx.enable = true — don't redefine it in modules/nginx.nix.
  # These two extend that auto-created vhost with the settings the old
  # /etc/nginx/sites-available/nextcloud config had (large upload size, HSTS).
  services.nginx.virtualHosts."cloud.intrentaka.com".extraConfig = ''
    client_max_body_size 512M;
    add_header Strict-Transport-Security "max-age=15552000; includeSubDomains" always;
  '';
}
