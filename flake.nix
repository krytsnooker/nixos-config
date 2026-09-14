{
  description = "homeserver NixOS configuration";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
    nixpkgs-unstable.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-minecraft.url = "github:Infinidoge/nix-minecraft";
    nix-minecraft.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { self, nixpkgs, nixpkgs-unstable, nix-minecraft, ... }@inputs:
    let
      system = "x86_64-linux";
      unstablePkgs = nixpkgs-unstable.legacyPackages.${system};
    in
    {
      nixosConfigurations.homeserver = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        specialArgs = { inherit inputs unstablePkgs; };
        modules = [
          ./hosts/homeserver/configuration.nix
          ./modules/common.nix
          ./modules/desktop-kde.nix
          ./modules/lan-apps.nix
          ./modules/minecraft.nix
          ./modules/databases.nix
          ./modules/nginx.nix
          ./modules/samba.nix
          ./modules/emby.nix
          ./modules/nextcloud.nix
          ./modules/pihole-container.nix
          ./modules/xrdp.nix
          nix-minecraft.nixosModules.minecraft-servers
        ];
      };
    };
}
