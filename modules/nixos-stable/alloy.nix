{ config, pkgs, lib, ... }:

let
  jrn = "__journal__systemd_unit";
  src = [ jrn ];

  mkRule = { source_labels, target_label, regex ? null, replacement ? null }:
    let
      sl  = lib.concatMapStringsSep ", " (l: ''"${l}"'') source_labels;
      rx  = lib.optionalString (regex       != null) "\n    regex         = `${regex}`";
      rep = lib.optionalString (replacement != null) "\n    replacement   = \"${replacement}\"";
    in ''
        rule {
          source_labels = [${sl}]${rx}
          target_label  = "${target_label}"${rep}
        }
      '';

  mkSet = regex: label: value: mkRule {
    source_labels = src;
    regex         = regex;
    target_label  = label;
    replacement   = value;
  };

  mkGroup = regex: category: module:
    (mkSet regex "category" category) +
    (mkSet regex "module"   module);

  rulesPassthrough =
    (mkRule { source_labels = src;                              target_label = "unit";  }) +
    (mkRule { source_labels = [ "__journal_priority_keyword" ]; target_label = "level"; });

  rulesDefaults =
    (mkRule {
      source_labels = src;
      regex         = "(.+)\\.(service|scope)";
      target_label  = "service";
      replacement   = "$1";
    }) +
    (mkSet ".+" "category" "system")  +
    (mkSet ".+" "module"   "unknown");

  rulesKernel =
    (mkGroup "(systemd-journald|systemd-logind|systemd-modules-load|systemd-timesyncd|systemd-tmpfiles-setup|systemd-udevd|systemd-boot-random-seed)\\.service" "system" "kernel") +
    (mkSet "systemd-coredump@.+"             "category" "system")       +
    (mkSet "systemd-coredump@.+"             "module"   "kernel")       +
    (mkSet "systemd-coredump@.+"             "service"  "coredump")     +
    (mkSet "systemd-fsck@.+"                 "category" "system")       +
    (mkSet "systemd-fsck@.+"                 "module"   "kernel")       +
    (mkSet "systemd-fsck@.+"                 "service"  "fsck")         +
    (mkSet "init\\.scope"                    "category" "system")       +
    (mkSet "init\\.scope"                    "module"   "kernel")       +
    (mkSet "init\\.scope"                    "service"  "init")         +
    (mkSet "session-.+\\.(scope|service)"    "category" "system")       +
    (mkSet "session-.+\\.(scope|service)"    "module"   "kernel")       +
    (mkSet "session-.+\\.(scope|service)"    "service"  "session")      +
    (mkSet "user@.+\\.service"               "category" "system")       +
    (mkSet "user@.+\\.service"               "module"   "kernel")       +
    (mkSet "user@.+\\.service"               "service"  "user-session");

  rulesHardware =
    (mkGroup "(ModemManager|bluetooth|fwupd|fwupd-refresh|irqbalance|thermald|udisks2|fstrim)\\.service" "system" "hardware") +
    (mkSet "ModemManager\\.service"          "service" "modem-manager") +
    (mkSet "fwupd-refresh\\.service"         "service" "fwupd-refresh");

  rulesNetworking =
    (mkGroup "(NetworkManager|wpa_supplicant|avahi-daemon|samba-wsdd)\\.service" "system" "networking") +
    (mkSet "NetworkManager\\.service"        "service" "network-manager") +
    (mkSet "wpa_supplicant\\.service"        "service" "wpa-supplicant")  +
    (mkSet "avahi-daemon\\.service"          "service" "avahi");

  rulesDesktop =
    (mkGroup "(accounts-daemon|cups|cups-browsed|dbus|display-manager|polkit|rtkit-daemon)\\.service" "system" "desktop") +
    (mkGroup "drkonqi-coredump-processor@.+" "system" "desktop") +
    (mkSet "drkonqi-coredump-processor@.+"   "service" "drkonqi") +
    (mkSet "rtkit-daemon\\.service"          "service" "rtkit");

  rulesAcme =
    (mkGroup "acme-.+\\.service" "nixos-sys" "acme") +
    (mkSet "acme-order-renew-([^.]+)\\..+\\.service" "service" "renew-$1") +
    (mkSet "acme-(cloud\\..+)\\.service"              "service" "$1")       +
    (mkSet "acme-(vault\\..+)\\.service"              "service" "$1")       +
    (mkSet "acme-(intrentaka\\..+)\\.service"         "service" "$1")       +
    (mkSet "acme-setup\\.service"                     "service" "setup");

  rulesNix =
    (mkGroup "(nix-daemon|nixos-rebuild.+)\\.service" "nixos-sys" "nix") +
    (mkSet "nixos-rebuild.+\\.service" "service" "nixos-rebuild");

  rulesNixosSys =
    (mkGroup "(cron|logrotate-checkconf|nscd)\\.service" "nixos-sys" "system") +
    (mkSet "logrotate-checkconf\\.service" "service" "logrotate");

  rulesRemote =
    mkGroup "(xrdp|xrdp-sesman)\\.service" "nixos-sys" "remote";

  rulesNixosStable =
    mkGroup "(fail2ban|grafana|loki|alloy|murmur|mysql|postgresql|wireguard-wgsrv)\\.service" "nixos-modules" "nixos-stable";

  rulesNixosUnstable =
    (mkGroup "(vaultwarden|phpfpm-nextcloud|redis-nextcloud|nextcloud-setup|nextcloud-update-db|nextcloud-preview-gen)\\.service" "nixos-modules" "nixos-unstable") +
    (mkSet "phpfpm-nextcloud\\.service" "service" "nextcloud");

  rulesContainers =
    (mkGroup "podman-.+\\.service" "nixos-modules" "containers-podman") +
    (mkSet "podman-(.+)\\.service" "service" "$1");

  rulesServices =
    (mkGroup "(math-tutor|typing-tutor|music-info|music-scanner|rom-transfer|mc-control|host-filepaths|minecraft-server-.+|mjh-proxy|nginx|lan-pastebin)\\.service" "nixos-modules" "services") +
    (mkSet "minecraft-server-.+\\.service" "service" "minecraft");

  allRules =
    rulesPassthrough   +
    rulesDefaults      +
    rulesKernel        +
    rulesHardware      +
    rulesNetworking    +
    rulesDesktop       +
    rulesAcme          +
    rulesNix           +
    rulesNixosSys      +
    rulesRemote        +
    rulesNixosStable   +
    rulesNixosUnstable +
    rulesContainers    +
    rulesServices;

  alloyConfig = pkgs.writeText "homeserver-alloy.alloy" ''
    loki.write "local" {
      endpoint {
        url = "http://${config.homeserver.localhost}:${toString config.homeserver.ports.loki}/loki/api/v1/push"
      }
    }

    loki.relabel "journal" {
      forward_to = []

    ${allRules}
    }

    loki.source.journal "read" {
      relabel_rules = loki.relabel.journal.rules
      forward_to    = [loki.write.local.receiver]
      max_age       = "12h"
      labels        = {
        job  = "systemd-journal",
        host = "${config.networking.hostName}",
      }
    }
  '';

in
{
  # ---------------------------------------------------------------------------
  # Alloy — ships journald → Loki
  # ---------------------------------------------------------------------------

  services.alloy = {
    enable     = true;
    configPath = alloyConfig;
    extraFlags = [
      "--server.http.listen-addr=${config.homeserver.localhost}:${toString config.homeserver.ports.alloy}"
    ];
  };

  systemd.services.alloy.serviceConfig = config.homeserver.sandbox;
}
