{
  description = "NixOS configs: test workstation + homeserver migration target";

  inputs = {
    # TODO: bump this to whatever the current NixOS stable release is when
    # you actually install (check nixos.org). This needs to be new enough to
    # ship a nextcloud package >= 34.x — your live instance is on 34.0.3 and
    # Nextcloud cannot open a data directory with an older server version
    # (no downgrades). Verify on search.nixos.org before relying on it.
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";

    # Community flake providing pinned historical Minecraft server versions
    # (mainline nixpkgs only ships whatever single version is current).
    nix-minecraft.url = "github:Infinidoge/nix-minecraft";
    nix-minecraft.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { self, nixpkgs, nix-minecraft, ... }@inputs:
    let
      system = "x86_64-linux";
      commonModules = [
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
    in
    {
      nixosConfigurations = {
        # Test box: i7-12700K, 32GB RAM. Build/validate the config here first.
        workstation = nixpkgs.lib.nixosSystem {
          inherit system;
          specialArgs = { inherit inputs; };
          modules = [ ./hosts/workstation/configuration.nix ] ++ commonModules;
        };

        # Real target: HP ProDesk G4 800, i5-7500T, 8GB RAM, 256GB SSD + SAS DAS.
        homeserver = nixpkgs.lib.nixosSystem {
          inherit system;
          specialArgs = { inherit inputs; };
          modules = [ ./hosts/homeserver/configuration.nix ] ++ commonModules;
        };
      };
    };
}
