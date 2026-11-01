{ config, pkgs, lib, ... }:

{
  # Real nixpkgs brave-origin package, pulled from nixpkgs-unstable (see
  # flake.nix overlay) — added alongside the existing custom
  # brave-origin-nightly derivation (modules/pkgs/brave-origin-nightly.nix)
  # for comparison before deciding whether to replace it.
  environment.systemPackages = [ pkgs.brave-origin ];

  security.wrappers.brave-origin-sandbox = {
    source = "${pkgs.brave-origin}/opt/brave.com/brave-origin/chrome-sandbox";
    owner  = "root";
    group  = "root";
    setuid = true;
  };
}
