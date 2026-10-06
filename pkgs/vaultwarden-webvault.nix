{
  lib,
  buildNpmPackage,
  fetchFromGitHub,
  python3,
  dart-sass,
}:

buildNpmPackage rec {
  pname = "vaultwarden-webvault";
  version = "2026.7.0+0";

  src = fetchFromGitHub {
    owner = "vaultwarden";
    repo = "vw_web_builds";
    tag = "v${version}";
    hash = "sha256-yX6Lf+7q9ptVAlx0q0Z55GnKhvAIjYvvvKR1ZZroWeo=";
  };

  npmDepsFetcherVersion = 2;
  npmDepsHash = "sha256-WRxlvkgWboO0ukUHgjC5CrfgfwnmUfDXI4r5dx9CKww=";

  nativeBuildInputs = [
    python3
    dart-sass
  ];

  makeCacheWritable = true;

  env = {
    ELECTRON_SKIP_BINARY_DOWNLOAD = "1";
    npm_config_build_from_source = "true";
  };

  preBuild = ''
    echo "export const compilerCommand = ['dart-sass'];" > node_modules/sass-embedded/dist/lib/src/compiler-path.js
  '';

  npmRebuildFlags = [ "--ignore-scripts" ];
  npmBuildScript = "dist:oss:selfhost";
  npmBuildFlags = [ "--workspace" "apps/web" ];
  npmFlags = [ "--legacy-peer-deps" ];

  installPhase = ''
    runHook preInstall
    mkdir -p $out/share/vaultwarden
    mv apps/web/build $out/share/vaultwarden/vault
    runHook postInstall
  '';

  meta = {
    description = "Web vault for vaultwarden";
    homepage = "https://github.com/vaultwarden/vw_web_builds";
    platforms = lib.platforms.all;
    license = lib.licenses.gpl3Plus;
  };
}
