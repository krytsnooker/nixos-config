#!/usr/bin/env bash
# Verifies systemd units are healthy and service ports/vhosts respond.
# Ports mirror config-values.nix as of writing — update both together.
# Run on the homeserver itself.
set -uo pipefail

pass=0
fail=0

ok()   { echo "  OK   $1"; pass=$((pass+1)); }
bad()  { echo "  FAIL $1"; fail=$((fail+1)); }
section() { echo; echo "== $1 =="; }

section "Failed systemd units"
failed=$(systemctl --failed --no-legend --plain)
if [ -z "$failed" ]; then
  ok "no failed units"
else
  while IFS= read -r line; do
    bad "unit failed: $line"
  done <<< "$failed"
fi

section "Core service units active"
for unit in nginx podman-pihole podman-emby podman-audiobookshelf \
            phpfpm-nextcloud redis-nextcloud murmur vaultwarden flood \
            qbittorrent grafana loki alloy wg wireguard-wg0 \
            typing-tutor math-tutor lan-pastebin music-info \
            rom-transfer mc-control; do
  state=$(systemctl is-active "$unit" 2>/dev/null)
  if [ "$state" = "active" ]; then
    ok "$unit is active"
  else
    bad "$unit is $state"
  fi
done

section "HTTP endpoints (localhost, ports from config-values.nix)"
check_http() {
  local name="$1" port="$2" path="${3:-/}"
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "http://127.0.0.1:$port$path" 2>/dev/null)
  if [[ "$code" =~ ^(2|3)[0-9][0-9]$ ]]; then
    ok "$name (:$port$path) -> HTTP $code"
  else
    bad "$name (:$port$path) -> HTTP ${code:-no response}"
  fi
}
check_http homepage       5000
check_http lan-pastebin   5001
check_http typing-tutor   3000
check_http math-tutor     3001
check_http music-info     5010
check_http rom-transfer   5035
check_http mc-control     5020
check_http emby           8096
check_http audiobookshelf 13378
check_http grafana        3010
check_http flood          8091
check_http pihole-admin   8083 /admin/

section "External vhosts (nginx, TLS, by Host header)"
check_vhost() {
  local host="$1"; shift
  local expected=("$@")
  local code
  code=$(curl -sk -o /dev/null -w '%{http_code}' --max-time 5 -H "Host: $host" "https://127.0.0.1/" 2>/dev/null)
  for e in "${expected[@]}"; do
    if [ "$code" = "$e" ]; then
      ok "$host -> HTTP $code"
      return
    fi
  done
  bad "$host -> HTTP ${code:-no response} (expected one of: ${expected[*]})"
}
check_vhost intrentaka.com 200 301 302
check_vhost www.intrentaka.com 200 301 302
check_vhost vault.intrentaka.com 200
# cloud.intrentaka.com is intentionally blocked (see nextcloud.nix: CVE-2026-109618/109619) — 503 is expected, not a failure
check_vhost cloud.intrentaka.com 503

section "Pi-hole DNS resolution"
if command -v dig >/dev/null 2>&1; then
  if dig +short +time=3 @127.0.0.1 google.com >/dev/null 2>&1; then
    ok "DNS query via Pi-hole succeeded"
  else
    bad "DNS query via Pi-hole failed"
  fi
elif command -v nslookup >/dev/null 2>&1; then
  if nslookup -timeout=3 google.com 127.0.0.1 >/dev/null 2>&1; then
    ok "DNS query via Pi-hole succeeded"
  else
    bad "DNS query via Pi-hole failed"
  fi
elif command -v python3 >/dev/null 2>&1; then
  if python3 - <<'PYEOF' >/dev/null 2>&1
import socket
q = b"\xaa\xaa\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00\x06google\x03com\x00\x00\x01\x00\x01"
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(3)
s.sendto(q, ("127.0.0.1", 53))
data, _ = s.recvfrom(512)
assert len(data) > len(q)
PYEOF
  then
    ok "DNS query via Pi-hole succeeded"
  else
    bad "DNS query via Pi-hole failed"
  fi
else
  echo "  NOTE no dig/nslookup/python3 available, skipping DNS check"
fi

echo
echo "== Summary: $pass OK, $fail FAIL =="
[ "$fail" -eq 0 ]
