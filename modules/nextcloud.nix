{ config, pkgs, lib, ... }:

{
  services.nextcloud = {
    enable = true;
    package = pkgs.nextcloud33;
    hostName = "cloud.intrentaka.com";
    https = true; # controls PHP-FPM's HTTPS flag + module's own HSTS header — NOT forceSSL/enableACME, that's set explicitly below
    database.createLocally = true;
    configureRedis = true;
    config = {
      dbtype = "mysql";
      adminuser = "admin";
      adminpassFile = "/var/lib/nextcloud-admin-pass";
    };
    settings = {
      trusted_proxies = [ "127.0.0.1" ];
    };
  };

  # The module creates services.nginx.virtualHosts.${hostName} itself but
  # does NOT enable SSL/ACME on it automatically (confirmed by reading the
  # actual module source) — set explicitly here, merged into that same vhost.
  services.nginx.virtualHosts."cloud.intrentaka.com" = {
    forceSSL = true;
    enableACME = true;
    extraConfig = ''
    '';
  };
}
