{ lib, stdenv, fetchurl, dpkg, autoPatchelfHook, makeWrapper,
  glib, nss, nspr, at-spi2-atk, at-spi2-core, atk,
  dbus, cups, expat, xorg, libxkbcommon, alsa-lib,
  mesa, libGL, cairo, pango, systemd, gcc }:

stdenv.mkDerivation rec {
  pname = "brave-origin-nightly";
  version = "1.96.44";

  src = fetchurl {
    url = "https://brave-browser-apt-nightly.s3.brave.com/pool/main/b/brave-origin-nightly/${pname}_${version}_amd64.deb";
    hash = "sha256-qG4WMwO9AGREkvk2r0BRVWDhF+XYgvWgm7lwYz9P5XM=";
  };

  nativeBuildInputs = [ dpkg autoPatchelfHook makeWrapper ];

  buildInputs = [
    glib nss nspr atk at-spi2-atk at-spi2-core
    dbus cups expat xorg.libxcb libxkbcommon alsa-lib
    mesa libGL xorg.libX11 xorg.libXext cairo pango
    systemd xorg.libXcomposite xorg.libXdamage
    xorg.libXfixes xorg.libXrandr gcc.cc.lib
  ];

  autoPatchelfIgnoreMissingDeps = [
    "libQt5Core.so.5" "libQt5Gui.so.5" "libQt5Widgets.so.5"
    "libQt6Core.so.6" "libQt6Gui.so.6" "libQt6Widgets.so.6"
  ];

  unpackPhase = ''
    dpkg-deb --fsys-tarfile $src | tar x --no-same-permissions --no-same-owner
  '';

  installPhase = ''
    runHook preInstall

    appdir=$out/opt/brave.com/brave-origin-nightly
    mkdir -p "$appdir"
    cp -r opt/brave.com/brave-origin-nightly/. "$appdir/"

    mkdir -p $out/share/applications
    cp usr/share/applications/brave-origin-nightly.desktop $out/share/applications/
    substituteInPlace $out/share/applications/brave-origin-nightly.desktop \
      --replace-fail "Exec=/usr/bin/brave-origin-nightly" "Exec=$out/bin/brave-origin-nightly"

    mkdir -p $out/share/man/man1
    cp usr/share/man/man1/brave-origin-nightly.1.gz $out/share/man/man1/ || true

    mkdir -p $out/bin
    makeWrapper "$appdir/brave" "$out/bin/brave-origin-nightly" \
      --set CHROME_VERSION_EXTRA nightly \
      --set GNOME_DISABLE_CRASH_DIALOG SET_BY_GOOGLE_CHROME \
      --add-flags "--password-store=basic" \
      --prefix LD_LIBRARY_PATH : ${lib.makeLibraryPath [ libGL mesa ]}

    runHook postInstall
  '';

  meta = with lib; {
    description = "Brave Origin privacy-focused browser";
    homepage = "https://brave.com/origin/";
    license = licenses.mpl20;
    sourceProvenance = [ sourceTypes.binaryNativeCode ];
    platforms = [ "x86_64-linux" ];
    mainProgram = "brave-origin-nightly";
  };
}
