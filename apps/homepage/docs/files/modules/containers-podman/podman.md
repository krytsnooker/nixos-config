# modules/containers-podman/podman.nix

Enables Podman and creates all Podman service user accounts. This is the shared base module that all container modules depend on.

## Podman setup

- `virtualisation.podman.enable = true`
- `dockerCompat = true` — provides a `docker` CLI shim so Docker-targeted tooling works
- `virtualisation.oci-containers.backend = "podman"` — directs all `oci-containers` definitions to use Podman

## User/group creation

All Podman service users are created from a single `lib.mapAttrs` loop over `config.homeserver.uids.podman`:

```nix
users.users = lib.mapAttrs (name: uid: {
  isSystemUser = true;
  inherit uid;
  group = name;
}) config.homeserver.uids.podman;

users.groups = lib.mapAttrs (name: _: { }) config.homeserver.uids.podman;
```

This means adding a new Podman container only requires:
1. Adding a `name = uid` entry to `uids.podman` in `config-values.nix`
2. Creating the container definition in a new module under `containers-podman/`

The individual container modules then extend the auto-created user with `extraGroups` as needed (e.g., `emby.nix` adds `video` and `sambashare`).

## Options declared

`homeserver.podman.tmpfiles.mode` and `homeserver.podman.tmpfiles.age` — defaults used by all container modules when creating state directories via `systemd.tmpfiles.rules`. Centralising these means changing the default mode for all container state dirs is a one-line change here.
