#!/usr/bin/env bash
# Run this ON THE OLD LIVE SERVER (as root, or it'll re-exec itself with sudo)
# to collect the high-value info for the NixOS migration into one file.
#
# Usage:
#   sudo bash collect-migration-info.sh
#
# Output: ./nixos-migration-info.txt in the current directory.
#
# This script tries to redact obvious secrets (Nextcloud passwordsalt/secret/
# dbpassword, etc). Skim the output before pasting it anywhere — if anything
# looks like a password, token, or key, strip it yourself first.

set -u

if [ "$(id -u)" -ne 0 ]; then
  echo "Re-running with sudo..."
  exec sudo bash "$0" "$@"
fi

OUT="./nixos-migration-info.txt"
: > "$OUT"

section() {
  {
    echo ""
    echo "=================================================================="
    echo "== $1"
    echo "=================================================================="
  } >> "$OUT"
}

run() {
  # run <description> <command...>
  local desc="$1"; shift
  {
    echo ""
    echo "--- $desc ---"
    echo "\$ $*"
  } >> "$OUT"
  if command -v "$1" >/dev/null 2>&1; then
    "$@" >> "$OUT" 2>&1
  else
    echo "(command not found: $1)" >> "$OUT"
  fi
}

# ---------------------------------------------------------------------------
section "1. Users / UIDs / GIDs"
# ---------------------------------------------------------------------------
{
  echo ""
  echo "--- Regular user accounts (UID 1000-59999) ---"
  awk -F: '($3>=1000 && $3<60000) {print}' /etc/passwd
  echo ""
  echo "--- Groups for each of those users ---"
  awk -F: '($3>=1000 && $3<60000) {print $1}' /etc/passwd | while read -r u; do
    echo "$u: $(id "$u" 2>&1)"
  done
} >> "$OUT"

# ---------------------------------------------------------------------------
section "2. Disks / DAS / Samba"
# ---------------------------------------------------------------------------
run "blkid" blkid
run "lsblk -f" lsblk -f
{
  echo ""
  echo "--- /etc/fstab ---"
  cat /etc/fstab 2>&1
} >> "$OUT"
{
  echo ""
  echo "--- /etc/samba/smb.conf ---"
  cat /etc/samba/smb.conf 2>&1
} >> "$OUT"
run "df -h" df -h

# ---------------------------------------------------------------------------
section "3. Emby hardware transcoding"
# ---------------------------------------------------------------------------
{
  echo ""
  echo "--- /dev/dri (presence = GPU accel device nodes exist) ---"
  ls -la /dev/dri 2>&1
} >> "$OUT"
run "vainfo (VAAPI capability check, if installed)" vainfo

for candidate in \
  /var/lib/emby/config/encoding.xml \
  /var/snap/emby-server/current/config/encoding.xml \
  "$HOME/.local/share/emby-server/config/encoding.xml"
do
  if [ -f "$candidate" ]; then
    {
      echo ""
      echo "--- $candidate ---"
      cat "$candidate"
    } >> "$OUT"
  fi
done
{
  echo ""
  echo "--- Searching for any encoding.xml under common Emby dirs (fallback) ---"
  find /var/lib/emby /var/snap/emby-server /opt/emby* -iname 'encoding.xml' 2>/dev/null
} >> "$OUT"

# ---------------------------------------------------------------------------
section "4. Cron jobs / scheduled tasks"
# ---------------------------------------------------------------------------
{
  echo ""
  echo "--- root crontab ---"
  crontab -l -u root 2>&1
  echo ""
  echo "--- other users' crontabs ---"
  awk -F: '($3>=1000 && $3<60000) {print $1}' /etc/passwd | while read -r u; do
    echo "# crontab for $u:"
    crontab -l -u "$u" 2>&1
  done
  echo ""
  echo "--- /etc/crontab ---"
  cat /etc/crontab 2>&1
  echo ""
  echo "--- /etc/cron.d/ ---"
  for f in /etc/cron.d/*; do
    [ -f "$f" ] && { echo "# $f"; cat "$f"; }
  done
} >> "$OUT"
run "systemd timers" systemctl list-timers --all --no-pager

# ---------------------------------------------------------------------------
section "5. Nextcloud (snap) version / apps / config"
# ---------------------------------------------------------------------------
NEXTCLOUD_OCC=""
if command -v nextcloud.occ >/dev/null 2>&1; then
  NEXTCLOUD_OCC="nextcloud.occ"
elif command -v occ >/dev/null 2>&1; then
  NEXTCLOUD_OCC="occ"
fi

if [ -n "$NEXTCLOUD_OCC" ]; then
  run "Nextcloud status/version" "$NEXTCLOUD_OCC" status
  run "Nextcloud installed apps" "$NEXTCLOUD_OCC" app:list
  {
    echo ""
    echo "--- Nextcloud system config (secrets redacted) ---"
    "$NEXTCLOUD_OCC" config:list system 2>&1 | \
      grep -viE '"(passwordsalt|secret|dbpassword|instanceid|updater\.secret\.key|mail_smtppassword)"'
  } >> "$OUT"
else
  echo "(nextcloud.occ / occ not found on PATH — check the exact snap command, e.g. 'snap run nextcloud.occ')" >> "$OUT"
fi

echo ""
echo "Done. Review $OUT for anything sensitive before sharing it, then paste its contents here."
