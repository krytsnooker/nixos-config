{
  description = "homeserver NixOS configuration";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    nixpkgs-unstable.url = "github:NixOS/nixpkgs/nixos-unstable";
    nix-minecraft.url = "github:Infinidoge/nix-minecraft";
    nix-minecraft.inputs.nixpkgs.follows = "nixpkgs";
    vpn-confinement.url = "github:Maroka-chan/VPN-Confinement";
  };

  outputs = { self, nixpkgs, nixpkgs-unstable, nix-minecraft, vpn-confinement }: {
    nixosConfigurations.homeserver = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      specialArgs = { inherit self nix-minecraft; };

      modules = [
        vpn-confinement.nixosModules.default

        {
          nixpkgs.overlays = [ nix-minecraft.overlays.default ];
          nixpkgs.config.allowUnfree = true;
          _module.args.pkgsUnstable = nixpkgs-unstable.legacyPackages.x86_64-linux;
        }

        ./hardware-configuration.nix
        ./configuration.nix

        ./config-values.nix
        ./modules/nginx.nix

        ./modules/nixos-stable/databases.nix
        ./modules/nixos-stable/fail2ban.nix
        ./modules/nixos-stable/grafana.nix
        ./modules/nixos-stable/loki.nix
        ./modules/nixos-stable/mumble.nix
        ./modules/nixos-stable/alloy.nix
        ./modules/nixos-stable/qbittorrent.nix
        ./modules/nixos-stable/systempackages.nix
        ./modules/nixos-stable/vpn-namespace.nix
        ./modules/nixos-stable/wireguard.nix

        ./modules/nixos-unstable/nextcloud.nix
        ./modules/nixos-unstable/vaultwarden.nix
        ./modules/nixos-unstable/brave-origin.nix

        ./modules/containers-podman/podman.nix
        ./modules/containers-podman/emby.nix
        ./modules/containers-podman/audiobookshelf.nix
        ./modules/containers-podman/pihole.nix

        ./modules/services/lan-apps.nix
        ./modules/services/host-filepaths.nix
        ./modules/services/minecraft.nix
      ];
    };
  };
}
