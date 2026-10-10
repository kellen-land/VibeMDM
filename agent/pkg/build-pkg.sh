#!/bin/bash
# Build a macOS installer package for the VibeMDM agent. Run on a Mac.
#
#   ./build-pkg.sh --server https://mdm.example.com --token ENROLLMENT_TOKEN [--version 1.0.0] [--sign "Developer ID Installer: Name (TEAMID)"]
#
# The resulting VibeMDM-Agent-<version>.pkg installs the universal binary to
# /usr/local/bin/vibemdm-agent and enrolls the Mac (with --mdm) on install.
# A pkg signed with a Developer ID Installer certificate can also be deployed
# through MDM with the InstallEnterpriseApplication command.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
SERVER=""; TOKEN=""; VERSION="${VERSION:-1.0.0}"; SIGN=""
while [ $# -gt 0 ]; do
  case "$1" in
    --server) SERVER="$2"; shift 2;;
    --token) TOKEN="$2"; shift 2;;
    --version) VERSION="$2"; shift 2;;
    --sign) SIGN="$2"; shift 2;;
    *) echo "unknown argument $1"; exit 2;;
  esac
done
[ -n "$SERVER" ] && [ -n "$TOKEN" ] || { echo "--server and --token are required"; exit 2; }
command -v pkgbuild >/dev/null || { echo "pkgbuild not found; run on macOS"; exit 1; }

ARM="$ROOT/bin/agents/vibemdm-agent-darwin-arm64"
AMD="$ROOT/bin/agents/vibemdm-agent-darwin-amd64"
if [ ! -f "$ARM" ] || [ ! -f "$AMD" ]; then
  echo "Building agent binaries…"; (cd "$ROOT" && make agent)
fi
WORK="$(mktemp -d)"
mkdir -p "$WORK/root/usr/local/bin" "$WORK/scripts"
lipo -create -output "$WORK/root/usr/local/bin/vibemdm-agent" "$ARM" "$AMD"
chmod 755 "$WORK/root/usr/local/bin/vibemdm-agent"
if [ -n "$SIGN" ]; then
  codesign --force --options runtime --timestamp --sign "${SIGN/Installer/Application}" "$WORK/root/usr/local/bin/vibemdm-agent" || true
fi
cat > "$WORK/scripts/postinstall" <<POST
#!/bin/bash
/usr/local/bin/vibemdm-agent enroll --server "$SERVER" --token "$TOKEN" --mdm || exit 1
exit 0
POST
chmod 755 "$WORK/scripts/postinstall"
OUT="$ROOT/bin/VibeMDM-Agent-$VERSION.pkg"
ARGS=(--root "$WORK/root" --scripts "$WORK/scripts" --identifier com.vibemdm.agent --version "$VERSION" --install-location /)
if [ -n "$SIGN" ]; then ARGS+=(--sign "$SIGN"); fi
pkgbuild "${ARGS[@]}" "$OUT"
rm -rf "$WORK"
echo "Built $OUT"
[ -n "$SIGN" ] && echo "Notarize with: xcrun notarytool submit \"$OUT\" --keychain-profile <profile> --wait && xcrun stapler staple \"$OUT\""
