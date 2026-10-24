{
  description = "homeserver NixOS configuration";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
    nixpkgs-unstable.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-minecraft.url = "github:Infinidoge/nix-minecraft";
    nix-minecraft.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { self, nixpkgs, nixpkgs-unstable, nix-minecraft }: {
    nixosConfigurations.homeserver = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      specialArgs = { inherit self; };

      modules = [
        {
          nixpkgs.overlays = [
            (final: prev: {
              brave-origin-nightly = final.callPackage ./pkgs/brave-origin-nightly.nix {};
              inherit (nixpkgs-unstable.legacyPackages.x86_64-linux) nextcloud34 vaultwarden;
              vaultwarden-webvault = nixpkgs-unstable.legacyPackages.x86_64-linux.vaultwarden.webvault;
            })
            nix-minecraft.overlays.default
          ];
          nixpkgs.config.allowUnfree = true;
        }

        ./hardware-configuration.nix
        ./configuration.nix

        ./modules/config-values.nix
        ./modules/nginx.nix

        ./modules/pkgs/brave-origin-nightly.nix

        ./modules/nixos-stable/databases.nix
        ./modules/nixos-stable/fail2ban.nix
        ./modules/nixos-stable/grafana.nix
        ./modules/nixos-stable/loki.nix
        ./modules/nixos-stable/mumble.nix
        ./modules/nixos-stable/promtail.nix
        ./modules/nixos-stable/systempackages.nix

        ./modules/nixos-unstable/nextcloud.nix
        ./modules/nixos-unstable/vaultwarden.nix

        ./modules/containers-podman/podman.nix
        ./modules/containers-podman/emby.nix
        ./modules/containers-podman/audiobookshelf.nix
        ./modules/containers-podman/pihole.nix

        ./modules/services/lan-apps.nix
        nix-minecraft.nixosModules.minecraft-servers
        ./modules/services/minecraft.nix
      ];
    };
  };
}
