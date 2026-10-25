# modules/nixos-stable/fail2ban.nix

Configures fail2ban to protect nginx from brute-force and scanning attacks. Four jails watch nginx access and error logs.

## Jails

| Jail | Filter | Trigger | Action |
|---|---|---|---|
| `nginx-botsearch` | Built-in `nginx-botsearch` | Requests for non-existent paths (wordlist scanning) | Ban IP |
| `nginx-php-scan` | Custom `nginx-php-scan` | Requests for `.php` files (Nextcloud CVE scanners) | Ban IP |
| `nginx-http-auth` | Built-in `nginx-http-auth` | nginx 401 responses (credential stuffing) | Ban IP |
| `nginx-path-scan` | Custom `nginx-path-scan` | Requests for `/.env`, `/wp-admin`, `/setup.php`, etc. | Ban IP |

## Custom filters

**`nginx-php-scan`** — matches GET/HEAD/POST requests for any `.php` endpoint in the nginx access log. Catches automated scanners looking for Nextcloud, WordPress, and generic PHP vulnerabilities.

**`nginx-path-scan`** — matches requests for well-known attack paths: `.env`, `wp-admin`, `wp-login`, `setup.php`, `phpMyAdmin`, etc.

## Bantime increment

`bantime-increment` is enabled with a multiplier so repeat offenders receive exponentially longer bans. The base ban time is 10 minutes; subsequent bans double.

## Ignored addresses

LAN subnet (`192.168.0.0/24`) and the ISP-assigned IP range are in the ignore list so local devices and the upstream router are never banned.
