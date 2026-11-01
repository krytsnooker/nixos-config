{ config, pkgs, lib, ... }:

{
  # ---------------------------------------------------------------------------
  # fail2ban — bans IPs that trigger repeated nginx errors / bot scanning
  # ---------------------------------------------------------------------------

  environment.etc."fail2ban/filter.d/nginx-php-scan.conf".text = ''
    [Definition]
    failregex = ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) [^ ]*\.php[^ ]* HTTP/[0-9.]+" 404
    ignoreregex =
  '';

  environment.etc."fail2ban/filter.d/nginx-path-scan.conf".text = ''
    [Definition]
    failregex = ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) [^ ]*/\.env[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /config\.env[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /sendgrid\.env[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /env[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /\.git/[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /\.git-credentials[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /wp-(admin|login|config|content|includes|json|editor|themes|security|asyn|filemanager|filemannger|manager|seerting|gans|logis|whe|trbacks|blog_footer)[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /xmlrpc\.php[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /\.aws/[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /\.ssh/[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /\.docker/[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /\.netrc[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /(credentials|credentials\.json|credentials\.ini|secrets\.json|secrets\.env|service-account\.json|gcp-key\.json|gcp-credentials\.json|firebase-key\.json|key\.json|keyfile\.json|appsettings\.json|docker-compose\.yml|Dockerfile|\.npmrc|\.s3cfg|\.boto|\.pypirc|\.gitmodules)[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /(sa\.json|gcp-sa\.json|google-credentials\.json|google-key\.json|firebase-adminsdk\.json|application_default_credentials\.json)[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /\.config/(gcloud|rclone|sourcegraph|gitea|gitpod|coder)/[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) [^ ]*eval-stdin\.php[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) [^ ]*cgi-bin[^ ]*[%%.]2e[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) [^ ]*allow_url_include[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) [^ ]*invokefunction[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) [^ ]*(adminer|phpMyAdmin|phpmyadmin|pma)[^ ]*\.php[^ ]* HTTP[^"]*" [0-9]+
                ^<HOST> - \S+ \[.*?\] "(GET|POST|HEAD) /(\.gitlab-ci\.yml|\.github/|\.gitpod\.yml|bitrise\.yml|codefresh\.yml|\.kube/|\.cursor/)[^ ]* HTTP[^"]*" [0-9]+
    ignoreregex =
  '';

  services.fail2ban = {
    enable    = true;
    maxretry  = 5;
    bantime   = "24h";
    ignoreIP  = [
      "127.0.0.1/8"
      config.homeserver.lanSubnet
      config.homeserver.ispIp
    ];

    bantime-increment = {
      enable      = true;
      multipliers = "1 2 4 8 16 32 64";
      maxtime     = "168h";
    };

    jails = {
      nginx-botsearch = {
        settings = {
          filter   = "nginx-botsearch";
          logpath  = ''
            /var/log/nginx/access.log
             /var/log/nginx/access.log.1
          '';
          backend  = "auto";
          maxretry = 2;
          bantime  = "24h";
          findtime = "48h";
        };
      };

      nginx-php-scan = {
        settings = {
          filter   = "nginx-php-scan";
          logpath  = ''
            /var/log/nginx/access.log
             /var/log/nginx/access.log.1
          '';
          backend  = "auto";
          maxretry = 5;
          bantime  = "24h";
          findtime = "10m";
        };
      };

      nginx-http-auth = {
        settings = {
          filter   = "nginx-http-auth";
          logpath  = "/var/log/nginx/error.log";
          backend  = "auto";
          maxretry = 3;
          bantime  = "24h";
        };
      };

      nginx-path-scan = {
        settings = {
          filter   = "nginx-path-scan";
          logpath  = ''
            /var/log/nginx/access.log
             /var/log/nginx/access.log.1
          '';
          backend  = "auto";
          maxretry = 1;
          bantime  = "168h";
          findtime = "10m";
        };
      };
    };
  };

  systemd.services.fail2ban.serviceConfig = config.homeserver.sandbox;
}
