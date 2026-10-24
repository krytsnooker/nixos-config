{ config, pkgs, lib, ... }:

{
  environment.systemPackages = [ pkgs.brave-origin-nightly ];

  security.wrappers.brave-origin-nightly-sandbox = {
    source = "${pkgs.brave-origin-nightly}/opt/brave.com/brave-origin-nightly/chrome-sandbox";
    owner  = "root";
    group  = "root";
    setuid = true;
  };
}
