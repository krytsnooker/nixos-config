{ config, pkgs, lib, ... }:

{
  # ---------------------------------------------------------------------------
  # Options
  # ---------------------------------------------------------------------------

  options.homeserver.podman.tmpfiles = {
    mode = lib.mkOption { type = lib.types.str; default = "0755"; };
    age  = lib.mkOption { type = lib.types.str; default = "-"; };
  };

  config = {

    # -------------------------------------------------------------------------
    # Podman
    # -------------------------------------------------------------------------

    virtualisation.podman = {
      enable       = true;
      dockerCompat = true;
    };

    virtualisation.oci-containers.backend = "podman";

    # -------------------------------------------------------------------------
    # Users
    # -------------------------------------------------------------------------

    users.users = lib.mapAttrs (name: uid: {
      isSystemUser = true;
      inherit uid;
      group = name;
    }) config.homeserver.uids.podman;

    users.groups = lib.mapAttrs (name: _: { }) config.homeserver.uids.podman;
  };
}
