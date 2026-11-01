{ config, pkgs, lib, ... }:

let
  lokiUid = "homeserver-loki";

  cascadeVars = [
    {
      name       = "category";
      label      = "Category";
      type       = "query";
      datasource = { type = "loki"; uid = lokiUid; };
      query      = "label_values({host=\"homeserver\"}, category)";
      refresh    = 2;
      multi      = true;
      includeAll = true;
      allValue   = ".+";
    }
    {
      name       = "module";
      label      = "Module";
      type       = "query";
      datasource = { type = "loki"; uid = lokiUid; };
      query      = "label_values({host=\"homeserver\",category=~\"$category\"}, module)";
      refresh    = 2;
      multi      = true;
      includeAll = true;
      allValue   = ".+";
    }
    {
      name       = "service";
      label      = "Service";
      type       = "query";
      datasource = { type = "loki"; uid = lokiUid; };
      query      = "label_values({host=\"homeserver\",category=~\"$category\",module=~\"$module\"}, service)";
      refresh    = 2;
      multi      = true;
      includeAll = true;
      allValue   = ".+";
    }
  ];

  logsDashboard = {
    title         = "Homeserver Logs";
    uid           = "homeserver-logs";
    timezone      = "browser";
    schemaVersion = 37;
    refresh       = "30s";
    time          = { from = "now-1h"; to = "now"; };

    templating.list = cascadeVars ++ [{
      name       = "level";
      label      = "Level";
      type       = "custom";
      query      = "emerg,alert,crit,err,warning,notice,info,debug";
      multi      = true;
      includeAll = true;
      allValue   = ".+";
    }];

    panels = [{
      id         = 1;
      type       = "logs";
      title      = "Logs";
      gridPos    = { h = 24; w = 24; x = 0; y = 0; };
      datasource = { type = "loki"; uid = lokiUid; };
      targets    = [{
        expr  = "{host=\"homeserver\",category=~\"$category\",module=~\"$module\",service=~\"$service\",level=~\"$level\"}";
        refId = "A";
      }];
      options = {
        showTime       = true;
        showLabels     = true;
        wrapLogMessage = true;
        sortOrder      = "Descending";
        dedupStrategy  = "none";
      };
    }];
  };

  alertsDashboard = {
    title         = "Homeserver Alerts";
    uid           = "homeserver-alerts";
    timezone      = "browser";
    schemaVersion = 37;
    refresh       = "1m";
    time          = { from = "now-24h"; to = "now"; };

    templating.list = cascadeVars;

    panels = [{
      id      = 1;
      type    = "table";
      title   = "Errors & Critical by Service";
      gridPos = { h = 24; w = 24; x = 0; y = 0; };
      datasource = { type = "loki"; uid = lokiUid; };
      targets = [{
        expr         = "sum by (category, module, service) (count_over_time({host=\"homeserver\",category=~\"$category\",module=~\"$module\",service=~\"$service\",level=~\"err|crit|emerg|alert\"}[$__range]))";
        refId        = "A";
        instant      = true;
        legendFormat = "";
      }];
      transformations = [
        { id = "labelsToFields"; options = {}; }
        { id = "merge";          options = {}; }
        {
          id      = "organize";
          options.excludeByName = { Time = true; };
        }
        {
          id      = "sortBy";
          options.fields = [
            { displayName = "category"; desc = false; }
            { displayName = "module";   desc = false; }
            { displayName = "service";  desc = false; }
          ];
        }
      ];
      fieldConfig.defaults.links = [{
        title       = "View logs";
        url         = "/d/homeserver-logs/homeserver-logs?var-category=\${__data.fields.category}&var-module=\${__data.fields.module}&var-service=\${__data.fields.service}&var-level=err&var-level=crit&var-level=emerg&var-level=alert&from=\${__from}&to=\${__to}";
        targetBlank = false;
      }];
    }];
  };

  dashboardDir = pkgs.runCommand "grafana-dashboards" {} ''
    mkdir $out
    cp ${pkgs.writeText "homeserver-logs.json"   (builtins.toJSON logsDashboard)}   $out/homeserver-logs.json
    cp ${pkgs.writeText "homeserver-alerts.json" (builtins.toJSON alertsDashboard)} $out/homeserver-alerts.json
  '';
in

{
  # ---------------------------------------------------------------------------
  # Grafana — dashboards
  # ---------------------------------------------------------------------------

  services.grafana = {
    enable = true;
    settings = {
      server = {
        http_addr = "0.0.0.0";
        http_port = config.homeserver.ports.grafana;
      };
      analytics.reporting_enabled = false;
      security.secret_key = "$__file{/var/lib/grafana/secret_key}";
    };
    provision = {
      enable = true;
      datasources.settings = {
        deleteDatasources = [{ name = "Loki"; orgId = 1; }];
        datasources = [{
          name      = "Loki";
          type      = "loki";
          uid       = lokiUid;
          url       = "http://${config.homeserver.localhost}:${toString config.homeserver.ports.loki}";
          isDefault = true;
          access    = "proxy";
        }];
      };
      dashboards.settings.providers = [{
        name            = "homeserver";
        type            = "file";
        disableDeletion = true;
        options.path    = "${dashboardDir}";
      }];
    };
  };

  systemd.services.grafana.serviceConfig = config.homeserver.sandbox // {
    EnvironmentFile = "/var/lib/grafana/secrets.env";
  };

  networking.firewall.allowedTCPPorts = with config.homeserver.ports; [ grafana ];
}
