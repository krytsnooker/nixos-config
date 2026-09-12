{ config, lib, pkgs, modulesPath, ... }:

# PLACEHOLDER — this file gets OVERWRITTEN by the real output of:
#   nixos-generate-config --root /mnt
# run from the NixOS installer on the HP ProDesk G4 800, after partitioning
# the 256GB SSD. Do not hand-edit the UUIDs below; they're fake.
#
# Note: an earlier draft of this file assumed an HBA + SAS DAS was locally
# attached to this machine. The live server dump showed that's wrong — the
# DAS is on a separate machine (192.168.0.110) and this box just mounts it
# over CIFS (see hosts/homeserver/configuration.nix). No SAS/HBA kernel
# modules needed here.

{
  imports = [ (modulesPath + "/installer/scan/not-detected.nix") ];

  boot.initrd.availableKernelModules = [ "xhci_pci" "ahci" "usb_storage" "sd_mod" ];
  boot.initrd.kernelModules = [ ];
  boot.kernelModules = [ "kvm-intel" ];
  boot.extraModulePackages = [ ];

  fileSystems."/" = {
    device = "/dev/disk/by-uuid/REPLACE-ME-ROOT";
    fsType = "btrfs";
  };

  fileSystems."/home" = {
    device = "/dev/disk/by-uuid/REPLACE-ME-HOME";
    fsType = "btrfs";
  };

  fileSystems."/boot" = {
    device = "/dev/disk/by-uuid/REPLACE-ME-BOOT";
    fsType = "vfat";
  };

  nixpkgs.hostPlatform = "x86_64-linux";
  hardware.cpu.intel.updateMicrocode = lib.mkDefault true;
}
