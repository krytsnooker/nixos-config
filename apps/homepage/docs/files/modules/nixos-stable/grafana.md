# modules/nixos-stable/grafana.nix

Configures Grafana with two pre-provisioned dashboards for log viewing and alerting. Dashboards are defined as Nix attribute sets, serialised to JSON at build time, and loaded from the Nix store — no manual dashboard import required.

## Configuration

- Listens on `0.0.0.0:config.homeserver.ports.grafana` (3010)
- Analytics/telemetry reporting disabled
- Sandbox flags applied via `config.homeserver.sandbox`
- `EnvironmentFile` at `/var/lib/grafana/secrets.env` — holds `GF_SECURITY_ADMIN_PASSWORD` and `GF_SECURITY_SECRET_KEY`

## Datasource provisioning

A single Loki datasource is provisioned:

| Field | Value |
|---|---|
| Name | `Loki` |
| URL | `http://127.0.0.1:3100` |
| UID | `homeserver-loki` |
| Default | true |

`deleteDatasources` removes the old datasource entry on each rebuild to prevent stale duplicates.

## Dashboards

Both dashboards are defined entirely in Nix and compiled to JSON by `pkgs.runCommand`. The compiled JSON is written to the Nix store and referenced by the file provisioner.

### Homeserver Logs

- Cascading variables: `category → module → service` (each narrows the next via label_values queries)
- `level` variable: custom list of all syslog levels
- Single Loki logs panel spanning the full grid; shows time, labels, wrapped text; sorted descending
- Auto-refresh every 30 seconds; default window last 1 hour

### Homeserver Alerts

- Same cascading variables (no `level`)
- Table panel: counts `err|crit|emerg|alert` entries per `{category, module, service}` over the selected range
- Clickable links from each row → Homeserver Logs filtered to that service and error levels
- Auto-refresh every minute; default window last 24 hours

## Label taxonomy

Dashboards rely on Promtail assigning `host`, `category`, `module`, `service`, and `level` labels to every log stream. See `modules/nixos-stable/promtail.md` for the full relabeling ruleset.
