#!/usr/bin/env bash
# Partition, format, and mount the homeserver NVMe for a NixOS install.
# Run from the NixOS installer live environment as root.
#
# Usage:  bash partition-homeserver.sh [/dev/nvme0n1]
#
# REINSTALL MODE (default):
#   Formats with the same UUIDs as the existing hardware-configuration.nix,
#   so you can skip nixos-generate-config entirely — just clone the repo.
#
# NEW HARDWARE MODE:
#   Set PRESERVE_UUIDS=no to let mkfs assign fresh UUIDs, then run
#   nixos-generate-config --root /mnt and commit the updated file.

set -euo pipefail

DISK="${1:-/dev/nvme0n1}"
PRESERVE_UUIDS="${PRESERVE_UUIDS:-yes}"

# Known UUIDs from hardware-configuration.nix — only used when PRESERVE_UUIDS=yes
ESP_UUID="A745730E"                               # FAT serial (no hyphen)
ROOT_UUID="f2ba3164-49c0-4f90-bea6-8c7ada01d308"
HOME_UUID="8034dca1-d1fd-41fd-bc45-941c2e1ef74c"

ESP_SIZE="512MiB"
ROOT_END="60GiB"     # root ~60 GiB; home gets the rest (~190 GiB on 256 GB SSD)

# ── Pre-flight ────────────────────────────────────────────────────────────────
if [[ "$(id -u)" -ne 0 ]]; then
  echo "ERROR: must be run as root" >&2
  exit 1
fi

if [[ ! -b "$DISK" ]]; then
  echo "ERROR: $DISK is not a block device" >&2
  exit 1
fi

echo "Target disk: $DISK"
echo "Preserve UUIDs: $PRESERVE_UUIDS"
echo
lsblk "$DISK"
echo
read -rp "This will DESTROY all data on $DISK. Type YES to continue: " confirm
[[ "$confirm" == "YES" ]] || { echo "Aborted."; exit 1; }

# ── Partition ─────────────────────────────────────────────────────────────────
echo "==> Partitioning $DISK"
parted --script --align optimal "$DISK" \
  mklabel gpt \
  mkpart ESP fat32 1MiB "$ESP_SIZE" \
  set 1 esp on \
  mkpart root btrfs "$ESP_SIZE" "$ROOT_END" \
  mkpart home btrfs "$ROOT_END" 100%

partprobe "$DISK"
sleep 2

# ── Format ────────────────────────────────────────────────────────────────────
echo "==> Formatting"
if [[ "$PRESERVE_UUIDS" == "yes" ]]; then
  mkfs.fat  -F32 -n ESP -i "$ESP_UUID"               "${DISK}p1"
  mkfs.btrfs -f  -L nixos-root -U "$ROOT_UUID"       "${DISK}p2"
  mkfs.btrfs -f  -L nixos-home -U "$HOME_UUID"       "${DISK}p3"
else
  mkfs.fat  -F32 -n ESP                               "${DISK}p1"
  mkfs.btrfs -f  -L nixos-root                       "${DISK}p2"
  mkfs.btrfs -f  -L nixos-home                       "${DISK}p3"
  echo
  echo "NOTE: fresh UUIDs assigned. After mounting, run:"
  echo "  nixos-generate-config --root /mnt"
  echo "  and commit the updated hardware-configuration.nix."
  echo
fi

# ── Mount ─────────────────────────────────────────────────────────────────────
echo "==> Mounting"
mount "${DISK}p2" /mnt
mkdir -p /mnt/boot /mnt/home
mount "${DISK}p1" /mnt/boot
mount "${DISK}p3" /mnt/home

# ── Swapfile ──────────────────────────────────────────────────────────────────
echo "==> Creating 4 GB swapfile"
dd if=/dev/zero of=/mnt/swapfile bs=1M count=4096 status=progress
chmod 600 /mnt/swapfile
mkswap /mnt/swapfile
swapon /mnt/swapfile

# ── Summary ───────────────────────────────────────────────────────────────────
echo
echo "Done. Filesystem mounted at /mnt."
echo
echo "Next steps:"
echo "  1. git clone <repo-url> /mnt/etc/nixos"
if [[ "$PRESERVE_UUIDS" == "yes" ]]; then
  echo "  2. Create secrets files (see reference/reference.html)"
  echo "  3. nixos-install --flake /mnt/etc/nixos#homeserver"
else
  echo "  2. nixos-generate-config --root /mnt  (commit the result)"
  echo "  3. Create secrets files"
  echo "  4. nixos-install --flake /mnt/etc/nixos#homeserver"
fi
echo "  4. Reboot → passwd kryt → restore secrets → verify CIFS"
