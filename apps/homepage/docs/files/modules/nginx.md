# modules/nginx.nix

Configures nginx as the reverse proxy and TLS terminator for all external-facing domains. Also serves the LAN homepage as a static site.

## TLS / ACME

Let's Encrypt certificates are obtained for three domains via `security.acme`:

| Domain | Certificate used by |
|---|---|
| `intrentaka.com` | Emby reverse proxy vhost |
| `cloud.intrentaka.com` | Nextcloud vhost (defined in `nextcloud.nix`) |
| `vault.intrentaka.com` | Vaultwarden vhost (defined in `vaultwarden.nix`) |

`security.acme.defaults.email` is set here. ACME challenges go through nginx on port 80; certificates are then used for port 443 vhosts.

## Security headers

A shared `extraConfig` snippet is applied to every vhost that serves external traffic:

```
Strict-Transport-Security: max-age=31536000; includeSubDomains
X-Content-Type-Options: nosniff
X-Frame-Options: SAMEORIGIN
Referrer-Policy: strict-origin-when-cross-origin
```

## CORS headers (Emby)

CORS is configured on the Emby vhost to allow web player cross-origin requests. The `Access-Control-Allow-Origin` is set to `*` for media endpoints.

## Rate limiting (Emby auth)

The `/Users/AuthenticateByName` endpoint (Emby login) is rate-limited to prevent brute-force. A `limit_req_zone` is defined and applied to that `location` block only.

## Vhosts

| Vhost | Module | Notes |
|---|---|---|
| `intrentaka.com` + `www.intrentaka.com` | `nginx.nix` | Proxies to Emby at `localhost:8096`; forceSSL + ACME |
| `cloud.intrentaka.com` | `nextcloud.nix` | Managed by `services.nextcloud`; currently returns 503 (CVE block) |
| `vault.intrentaka.com` | `vaultwarden.nix` | Proxies to Vaultwarden at `localhost:8222`; WebSocket enabled |
| `homepage-lan` | `lan-apps.nix` | Static file serving from `apps/homepage/`; no TLS |

## Firewall

Opens ports 80 and 443.
