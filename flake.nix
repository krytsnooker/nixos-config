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

      modules = [
        {
          nixpkgs.overlays = [
            (final: prev: {
              brave-origin-nightly = final.callPackage ./pkgs/brave-origin-nightly.nix {};
              nextcloud34 = nixpkgs-unstable.legacyPackages.x86_64-linux.nextcloud34;
              vaultwarden = final.callPackage ./pkgs/vaultwarden.nix {
                inherit (nixpkgs-unstable.legacyPackages.x86_64-linux) rustPlatform;
              };
            })
            nix-minecraft.overlays.default
          ];
          nixpkgs.config.allowUnfree = true;
        }

        ./hardware-configuration.nix
        ./configuration.nix

        ./modules/lan-apps.nix
        ./modules/minecraft.nix
        ./modules/databases.nix
        ./modules/nginx.nix
        ./modules/emby.nix
        ./modules/nextcloud.nix
        ./modules/pihole-container.nix
        ./modules/audiobookshelf.nix
        ./modules/mumble.nix
        ./modules/mjh-proxy.nix
        ./modules/vaultwarden.nix
        nix-minecraft.nixosModules.minecraft-servers
      ];
    };
  };
}
