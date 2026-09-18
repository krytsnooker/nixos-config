{ config, pkgs, lib, ... }:

{
  services.murmur = {
    enable = true;
    openFirewall = true;
    welcometext = "Welcome to the homeserver Mumble!";
    bandwidth = 72000;
    users = 20;
    registerName = "hometalk";
  };
}
