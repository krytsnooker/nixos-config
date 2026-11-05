{ config, pkgs, pkgsUnstable, lib, ... }:

{
  # Real nixpkgs brave-origin package, pulled from nixpkgs-unstable (see
  # flake.nix _module.args.pkgsUnstable) — added alongside the existing custom
  # brave-origin-nightly derivation (modules/pkgs/brave-origin-nightly.nix)
  # for comparison before deciding whether to replace it.
  environment.systemPackages = [ pkgsUnstable.brave-origin ];

  security.wrappers.brave-origin-sandbox = {
    source = "${pkgsUnstable.brave-origin}/opt/brave.com/brave-origin/chrome-sandbox";
    owner  = "root";
    group  = "root";
    setuid = true;
  };
}
