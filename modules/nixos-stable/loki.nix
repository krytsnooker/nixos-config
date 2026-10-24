{ config, pkgs, lib, ... }:

{
  # ---------------------------------------------------------------------------
  # Loki — log storage
  # ---------------------------------------------------------------------------

  services.loki = {
    enable = true;
    configuration = {
      auth_enabled = false;

      server.http_listen_port = config.homeserver.ports.loki;

      ingester = {
        lifecycler = {
          address = config.homeserver.localhost;
          ring = {
            kvstore.store = "inmemory";
            replication_factor = 1;
          };
        };
        chunk_idle_period   = "5m";
        chunk_retain_period = "30s";
      };

      schema_config.configs = [{
        from         = "2024-01-01";
        store        = "tsdb";
        object_store = "filesystem";
        schema       = "v13";
        index = {
          prefix = "index_";
          period = "24h";
        };
      }];

      storage_config = {
        tsdb_shipper = {
          active_index_directory = "/var/lib/loki/tsdb-index";
          cache_location         = "/var/lib/loki/tsdb-cache";
        };
        filesystem.directory = "/var/lib/loki/chunks";
      };

      limits_config = {
        reject_old_samples         = true;
        reject_old_samples_max_age = "168h";
        retention_period           = "30d";
        retention_stream = [
          { selector = "{level=\"info\",category=\"system\"}"; priority = 3; period = "1d";  }
          { selector = "{level=\"info\",service=\"emby\"}";    priority = 2; period = "2d";  }
          { selector = "{level=\"info\"}";                     priority = 1; period = "7d";  }
          { selector = "{level=\"notice\"}";                   priority = 1; period = "7d";  }
          { selector = "{level=\"warning\"}";                  priority = 1; period = "14d"; }
        ];
      };

      compactor = {
        working_directory      = "/var/lib/loki/compactor";
        retention_enabled      = true;
        retention_delete_delay = "2h";
        delete_request_store   = "filesystem";
      };
    };
  };

  systemd.services.loki.serviceConfig = config.homeserver.sandbox;
}
