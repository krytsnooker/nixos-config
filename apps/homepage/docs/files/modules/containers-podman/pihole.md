# modules/containers-podman/pihole.nix

Defines the Pi-hole Podman container. Pi-hole serves as the LAN DNS resolver and ad blocker.

## Image

Pinned by digest: `pihole/pihole@sha256:7c96327e…`. Update deliberately.

## Port mapping

| Container port | Host port | Protocol | Purpose |
|---|---|---|---|
| 53 | 53 (`config.homeserver.ports.dns`) | TCP + UDP | DNS |
| `piholeInternal` (8080) | `pihole` (8083) | TCP | Web UI |

The internal port is pinned via `FTLCONF_webserver_port = piholeInternal`. This prevents Teleporter restores from resetting it back to the container default (80), which would break the port mapping.

## Volumes

| Host path | Container path | Purpose |
|---|---|---|
| `/var/lib/pihole/etc-pihole` | `/etc/pihole` | Configuration, DNS lists, database |
| `/var/lib/pihole/etc-dnsmasq.d` | `/etc/dnsmasq.d` | dnsmasq configuration |

## Environment

- `TZ` from `config.time.timeZone`
- `FTLCONF_dns_listeningMode = "ALL"` — listens on all interfaces so the container can answer queries from LAN clients (not just the host)
- `FTLCONF_webserver_port` pinned to prevent Teleporter resets
- `environmentFiles` references `/var/lib/pihole/secrets.env` for the admin password (`FTLCONF_webserver_api_password`)

## NET_ADMIN capability

`--cap-add=NET_ADMIN` is required by Pi-hole's FTL DNS engine to modify network settings.

## Custom DNS (split-horizon)

`/var/lib/pihole/etc-pihole/custom.list` maps external domains to `192.168.0.120` so LAN clients resolve `cloud.intrentaka.com`, `vault.intrentaka.com`, and `intrentaka.com` directly to the server without hairpin NAT (the ER605 does not support it). This file is **not** tracked in the NixOS config — it lives in the Pi-hole volume and must be recreated if `/var/lib/pihole/` is wiped.

After editing `custom.list`: `sudo systemctl restart podman-pihole.service`

## Firewall

Opens port `pihole` (8083) TCP and port `dns` (53) TCP + UDP.
