# modules/pkgs/brave-origin-nightly.nix

Installs the Brave Origin Nightly browser and creates the setuid sandbox wrapper it requires.

## Package

`pkgs.brave-origin-nightly` is defined in `./pkgs/brave-origin-nightly.nix` (the custom package derivation) and made available via the overlay in `flake.nix`. This module adds it to `environment.systemPackages`.

## Setuid sandbox

Chromium-based browsers require a setuid `chrome-sandbox` binary. NixOS does not set this automatically for custom packages. This module creates the wrapper explicitly:

```nix
security.wrappers.brave-origin-nightly-sandbox = {
  source = "${pkgs.brave-origin-nightly}/opt/brave.com/brave-origin-nightly/chrome-sandbox";
  owner  = "root";
  group  = "root";
  setuid = true;
};
```

The wrapper is placed at `/run/wrappers/bin/brave-origin-nightly-sandbox` at runtime.
