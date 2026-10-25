# flake.nix

The entry point for the entire NixOS configuration. Defines all external dependencies (inputs) and assembles the single `nixosConfigurations.homeserver` output.

## Inputs

| Input | Source | Purpose |
|---|---|---|
| `nixpkgs` | `github:NixOS/nixpkgs/nixos-25.11` | Primary package set; all stable modules draw from this |
| `nixpkgs-unstable` | `github:NixOS/nixpkgs/nixos-unstable` | Source for packages not yet stable: `nextcloud34`, `vaultwarden`, `vaultwarden-webvault` |
| `nix-minecraft` | `github:Infinidoge/nix-minecraft` | Provides `minecraft-servers` NixOS module and related packages; pinned to stable nixpkgs via `inputs.nixpkgs.follows` |

## Overlays

A single inline overlay is applied before any module loads:

- `brave-origin-nightly` — local package from `./pkgs/brave-origin-nightly.nix`
- `nextcloud34`, `vaultwarden`, `vaultwarden-webvault` — pulled from `nixpkgs-unstable.legacyPackages`
- `nix-minecraft.overlays.default` — makes nix-minecraft packages (NeoForge, Fabric, etc.) available in `pkgs`

`nixpkgs.config.allowUnfree = true` is set here because the overlay block is the first module evaluated.

## specialArgs

`self` and `nix-minecraft` are passed as `specialArgs` so they are available as function arguments in any module that declares them. `nix-minecraft` is needed by `modules/services/minecraft.nix` for its `imports` statement. `self` is used in `configuration.nix` to stamp `system.configurationRevision`.

## Module list order

The module list follows a fixed loading order: hardware → base config → shared options → networking → desktop packages → stable NixOS services → unstable NixOS services → Podman containers → custom systemd services. This order has no functional significance to Nix (all modules are merged) but reflects the dependency relationships and makes the list readable.
