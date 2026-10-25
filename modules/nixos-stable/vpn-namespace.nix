{ config, pkgs, lib, ... }:

{
  # ---------------------------------------------------------------------------
  # VPN namespace — WireGuard network namespace for service isolation
  # ---------------------------------------------------------------------------

  vpnNamespaces.wg = {
    enable              = true;
    wireguardConfigFile = "/var/lib/vpn/wg.conf";
    accessibleFrom      = [ config.homeserver.lanSubnet ];
  };

  boot.kernel.sysctl."net.ipv4.conf.all.forwarding" = 1;
}
