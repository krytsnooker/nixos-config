# modules/nixos-stable/loki.nix

Configures Loki as the log storage backend. Loki listens on localhost only; it is not exposed to the network. Alloy pushes logs to it; Grafana reads from it.

## Storage

TSDB index + filesystem chunks:

| Directory | Purpose |
|---|---|
| `/var/lib/loki/tsdb-index` | Active TSDB index |
| `/var/lib/loki/tsdb-cache` | TSDB cache |
| `/var/lib/loki/chunks` | Log chunk files |
| `/var/lib/loki/compactor` | Compactor working directory |

## Retention

Global retention is 30 days. Stream-level rules override this for high-volume or low-value streams:

| Selector | Priority | Retention |
|---|---|---|
| `{level="info", category="system"}` | 3 | 1 day |
| `{level="info", service="emby"}` | 2 | 2 days |
| `{level="info"}` | 1 | 7 days |
| `{level="notice"}` | 1 | 7 days |
| `{level="warning"}` | 1 | 14 days |

`err`, `crit`, `emerg`, and `alert` level logs are kept for the full 30 days (no override rule).

## Compactor

Retention deletion is deferred by 2 hours (`retention_delete_delay`) to allow in-flight queries to complete before chunks are removed.

## Hardening

`systemd.services.loki.serviceConfig = config.homeserver.sandbox` applies the standard sandbox flags.

## Notes

- `auth_enabled = false` — single-tenant; no authentication required between Alloy and Loki
- `replication_factor = 1` — single-node setup; no HA
- `reject_old_samples_max_age = "168h"` (7 days) — rejects log entries older than a week to prevent accidental backfill
