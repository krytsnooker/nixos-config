{
  lib,
  rustPlatform,
  fetchFromGitHub,
  pkg-config,
  openssl,
  libiconv,
  stdenv,
  sqlite,
  callPackage,
  dbBackend ? "sqlite_system",
  libmysqlclient,
  libpq,
}:

let
  webvault = callPackage ./vaultwarden-webvault.nix { };
in

rustPlatform.buildRustPackage (finalAttrs: {
  pname = "vaultwarden";
  version = "1.37.3";

  src = fetchFromGitHub {
    owner = "dani-garcia";
    repo = "vaultwarden";
    tag = finalAttrs.version;
    hash = "sha256-T2sTVsBCvsvgxjlTeBPSvA96mJ7TLYqLNvldCI73by0=";
  };

  cargoHash = "sha256-gUQxnGPo8jYTfG+Zsz8W35h8lkYDxI3mGnCdxNXYB4k=";

  env.VW_VERSION = finalAttrs.version;

  nativeBuildInputs = [ pkg-config ];
  buildInputs = [
    openssl
  ]
  ++ lib.optionals stdenv.hostPlatform.isDarwin [ libiconv ]
  ++ lib.optional (dbBackend == "mysql") libmysqlclient
  ++ lib.optional (dbBackend == "postgresql") libpq
  ++ lib.optional (dbBackend == "sqlite_system") sqlite;

  buildFeatures = dbBackend;

  passthru = {
    inherit webvault;
  };

  meta = {
    description = "Unofficial Bitwarden compatible server written in Rust";
    homepage = "https://github.com/dani-garcia/vaultwarden";
    changelog = "https://github.com/dani-garcia/vaultwarden/releases/tag/${finalAttrs.version}";
    license = lib.licenses.agpl3Only;
    mainProgram = "vaultwarden";
    platforms = lib.platforms.linux;
  };
})
