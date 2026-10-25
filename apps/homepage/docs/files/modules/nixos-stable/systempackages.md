# modules/nixos-stable/systempackages.nix

Installs desktop and developer tools into the system environment. All packages come from `pkgs` (stable nixpkgs), except `brave-origin-nightly` which is a local overlay package wired in `modules/pkgs/brave-origin-nightly.nix`.

## Installed packages

| Package | Purpose |
|---|---|
| `firefox` | Web browser |
| `vscode` | Code editor |
| `vim` | Terminal editor |
| `git` | Version control |
| `go` | Go compiler (used for rom-transfer local development) |
| `claude-code` | Claude Code CLI |
| `wget` | HTTP downloader |
| `curl` | HTTP client |
| `htop` | Process monitor |
| `ripgrep` | Fast grep |
| `neofetch` | System info display |
| `cifs-utils` | CIFS/SMB mount utilities (required for the DAS mounts in `configuration.nix`) |

## Notes

`brave-origin-nightly` is not listed here — it is added via `environment.systemPackages` in `modules/pkgs/brave-origin-nightly.nix` along with the required setuid sandbox wrapper.
