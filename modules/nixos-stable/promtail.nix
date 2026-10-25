{ config, pkgs, lib, ... }:

let
  # ---------------------------------------------------------------------------
  # Helpers
  # ---------------------------------------------------------------------------

  src = [ "__journal__systemd_unit" ];

  # Single relabel rule
  mkSet = regex: label: value: {
    source_labels = src;
    inherit regex;
    target_label  = label;
    replacement   = value;
  };

  # Two rules from one regex: sets category + module together
  mkGroup = regex: category: module: [
    (mkSet regex "category" category)
    (mkSet regex "module"   module)
  ];

  # ---------------------------------------------------------------------------
  # Relabeling rules — applied in order; later rules override earlier ones.
  # Each group sets category, module, and service for a class of units.
  # Defaults (system / unknown) are set first, then overridden by specifics.
  # ---------------------------------------------------------------------------

  # -- Passthrough: raw unit name and priority level -------------------------

  rulesPassthrough = [
    { source_labels = src;                              target_label = "unit";  }
    { source_labels = [ "__journal_priority_keyword" ]; target_label = "level"; }
  ];

  # -- Defaults: applied to everything; overridden below ---------------------

  rulesDefaults = [
    # service = unit name with .service/.scope stripped
    {
      source_labels = src;
      regex         = "(.+)\\.(service|scope)";
      target_label  = "service";
      replacement   = "$1";
    }
    (mkSet ".+" "category" "system")
    (mkSet ".+" "module"   "unknown")
  ];

  # -- system / kernel -------------------------------------------------------

  rulesKernel =
    (mkGroup "(systemd-journald|systemd-logind|systemd-modules-load|systemd-timesyncd|systemd-tmpfiles-setup|systemd-udevd|systemd-boot-random-seed)\\.service" "system" "kernel") ++
    (mkGroup "systemd-coredump@.+" "system" "kernel") ++ [ (mkSet "systemd-coredump@.+"       "service" "coredump")     ] ++
    (mkGroup "systemd-fsck@.+"     "system" "kernel") ++ [ (mkSet "systemd-fsck@.+"           "service" "fsck")         ] ++
    (mkGroup "init\\.scope"        "system" "kernel") ++ [ (mkSet "init\\.scope"              "service" "init")         ] ++
    (mkGroup "session-.+\\.(scope|service)" "system" "kernel") ++ [ (mkSet "session-.+\\.(scope|service)" "service" "session") ] ++
    (mkGroup "user@.+\\.service"   "system" "kernel") ++ [ (mkSet "user@.+\\.service"         "service" "user-session") ];

  # -- system / hardware -----------------------------------------------------

  rulesHardware =
    (mkGroup "(ModemManager|bluetooth|fwupd|fwupd-refresh|irqbalance|thermald|udisks2|fstrim)\\.service" "system" "hardware") ++
    [ (mkSet "ModemManager\\.service"  "service" "modem-manager") ] ++
    [ (mkSet "fwupd-refresh\\.service" "service" "fwupd-refresh") ];

  # -- system / networking ---------------------------------------------------

  rulesNetworking =
    (mkGroup "(NetworkManager|wpa_supplicant|avahi-daemon|samba-wsdd)\\.service" "system" "networking") ++
    [ (mkSet "NetworkManager\\.service" "service" "network-manager") ] ++
    [ (mkSet "wpa_supplicant\\.service" "service" "wpa-supplicant")  ] ++
    [ (mkSet "avahi-daemon\\.service"   "service" "avahi")           ];

  # -- system / desktop ------------------------------------------------------

  rulesDesktop =
    (mkGroup "(accounts-daemon|cups|cups-browsed|dbus|display-manager|polkit|rtkit-daemon)\\.service" "system" "desktop") ++
    (mkGroup "drkonqi-coredump-processor@.+" "system" "desktop") ++
    [ (mkSet "drkonqi-coredump-processor@.+" "service" "drkonqi") ] ++
    [ (mkSet "rtkit-daemon\\.service"        "service" "rtkit")   ];

  # -- nixos-sys / acme ------------------------------------------------------

  rulesAcme =
    (mkGroup "acme-.+\\.service" "nixos-sys" "acme") ++
    [
      # acme-order-renew-cloud.intrentaka.com.service → renew-cloud
      (mkSet "acme-order-renew-([^.]+)\\..+\\.service" "service" "renew-$1")
      # acme-cloud.intrentaka.com.service → cloud.intrentaka.com
      (mkSet "acme-(cloud\\..+)\\.service"              "service" "$1")
      (mkSet "acme-(vault\\..+)\\.service"              "service" "$1")
      (mkSet "acme-(intrentaka\\..+)\\.service"         "service" "$1")
      (mkSet "acme-setup\\.service"                     "service" "setup")
    ];

  # -- nixos-sys / nix -------------------------------------------------------

  rulesNix =
    (mkGroup "(nix-daemon|nixos-rebuild.+)\\.service" "nixos-sys" "nix") ++
    [ (mkSet "nixos-rebuild.+\\.service" "service" "nixos-rebuild") ];

  # -- nixos-sys / system ----------------------------------------------------

  rulesNixosSys =
    (mkGroup "(cron|logrotate-checkconf|nscd)\\.service" "nixos-sys" "system") ++
    [ (mkSet "logrotate-checkconf\\.service" "service" "logrotate") ];

  # -- nixos-sys / remote ----------------------------------------------------

  rulesRemote =
    mkGroup "(xrdp|xrdp-sesman)\\.service" "nixos-sys" "remote";

  # -- nixos-modules / nixos-stable ------------------------------------------

  rulesNixosStable =
    mkGroup "(fail2ban|flood|grafana|loki|promtail|murmur|mysql|postgresql|qbittorrent)\\.service" "nixos-modules" "nixos-stable";

  # -- nixos-modules / nixos-unstable ----------------------------------------

  rulesNixosUnstable =
    (mkGroup "(vaultwarden|phpfpm-nextcloud|redis-nextcloud|nextcloud-setup|nextcloud-update-db|nextcloud-preview-gen)\\.service" "nixos-modules" "nixos-unstable") ++
    [ (mkSet "phpfpm-nextcloud\\.service" "service" "nextcloud") ];

  # -- nixos-modules / containers-podman ------------------------------------

  rulesContainers =
    (mkGroup "podman-.+\\.service" "nixos-modules" "containers-podman") ++
    [ (mkSet "podman-(.+)\\.service" "service" "$1") ];

  # -- nixos-modules / services ----------------------------------------------

  rulesServices =
    (mkGroup "(math-tutor|typing-tutor|music-info|music-scanner|rom-transfer|mc-control|minecraft-server-.+|mjh-proxy|nginx|lan-pastebin)\\.service" "nixos-modules" "services") ++
    [ (mkSet "minecraft-server-.+\\.service" "service" "minecraft") ];

in

{
  # ---------------------------------------------------------------------------
  # Promtail — ships journald → Loki
  # ---------------------------------------------------------------------------

  services.promtail = {
    enable = true;
    configuration = {
      server = {
        http_listen_port = config.homeserver.ports.promtail;
        grpc_listen_port = 0;
      };

      clients = [{
        url = "http://${config.homeserver.localhost}:${toString config.homeserver.ports.loki}/loki/api/v1/push";
      }];

      scrape_configs = [{
        job_name = "journal";
        journal = {
          max_age = "12h";
          labels = {
            job  = "systemd-journal";
            host = "homeserver";
          };
        };
        relabel_configs =
          rulesPassthrough ++
          rulesDefaults    ++
          rulesKernel      ++
          rulesHardware    ++
          rulesNetworking  ++
          rulesDesktop     ++
          rulesAcme        ++
          rulesNix         ++
          rulesNixosSys    ++
          rulesRemote      ++
          rulesNixosStable ++
          rulesNixosUnstable ++
          rulesContainers  ++
          rulesServices;
      }];
    };
  };

  systemd.services.promtail.serviceConfig = config.homeserver.sandbox;
}
