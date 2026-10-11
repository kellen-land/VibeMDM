#!/bin/bash
# Build, sign and upload the VibeMDM agent package used for automatic installs through MDM.
#
# Run once on any Mac (and again after upgrading VibeMDM, so new Macs get the new agent):
#
#   VIBEMDM_URL=https://mdm.example.com VIBEMDM_API_TOKEN=<admin API token> ./build-agent-pkg.sh
#
# What it does:
#   1. downloads the agent binaries (Apple silicon and Intel) from your server;
#   2. builds a product archive with pkgbuild + productbuild; its postinstall copies the right
#      binary to /usr/local/bin and runs `vibemdm-agent enroll --from-mdm`;
#   3. signs it: makes a key and certificate request here, has VibeMDM's package-signing CA
#      sign it (the private key never leaves this Mac), then productsign with a throwaway
#      keychain. Set SIGN_IDENTITY to sign with your own Developer ID Installer instead;
#   4. uploads it to VibeMDM (Settings → Enrollment → Automatic agent install).
#
# Needs: macOS (pkgbuild, productbuild, productsign, security and openssl ship with it), curl.
set -euo pipefail

: "${VIBEMDM_URL:?set VIBEMDM_URL, e.g. https://mdm.example.com}"
: "${VIBEMDM_API_TOKEN:?set VIBEMDM_API_TOKEN to an admin API token (Settings → API tokens)}"
URL="${VIBEMDM_URL%/}"
[[ "$(uname -s)" == Darwin ]] || { echo "Run this on a Mac." >&2; exit 1; }

VERSION="${VERSION:-$(date -u +%Y.%m.%d.%H%M)}"
work="$(mktemp -d)"
keychain="$work/sign.keychain-db"
cleanup() {
	security delete-keychain "$keychain" >/dev/null 2>&1 || true
	rm -rf "$work"
}
trap cleanup EXIT
auth=(-H "Authorization: Bearer ${VIBEMDM_API_TOKEN}")

echo "==> Downloading agent binaries from $URL"
payload="$work/root/Library/Application Support/VibeMDM/pkg"
mkdir -p "$payload"
for arch in arm64 amd64; do
	curl -fsSL -o "$payload/vibemdm-agent-darwin-$arch" "$URL/agent/download/darwin-$arch"
	chmod 0755 "$payload/vibemdm-agent-darwin-$arch"
done

echo "==> Building the package (version $VERSION)"
mkdir -p "$work/scripts"
cat > "$work/scripts/postinstall" <<'POST'
#!/bin/bash
# Installed by MDM (InstallEnterpriseApplication). Puts the agent in place, then enrolls it
# with the one-time token from the VibeMDM bootstrap profile. Never fails the install: if
# enrollment does not complete, run the install command from Settings → Enrollment.
log=/Library/Logs/VibeMDM/install.log
mkdir -p /Library/Logs/VibeMDM /usr/local/bin
exec >>"$log" 2>&1
echo "$(date -u +%FT%TZ) postinstall starting"
src="/Library/Application Support/VibeMDM/pkg/vibemdm-agent-darwin-amd64"
[[ "$(uname -m)" == arm64 ]] && src="/Library/Application Support/VibeMDM/pkg/vibemdm-agent-darwin-arm64"
install -m 0755 -o root -g wheel "$src" /usr/local/bin/vibemdm-agent
if /usr/local/bin/vibemdm-agent enroll --from-mdm --wait 5m; then
	echo "$(date -u +%FT%TZ) agent enrolled"
else
	echo "$(date -u +%FT%TZ) agent enrollment did not complete"
fi
exit 0
POST
chmod 0755 "$work/scripts/postinstall"
pkgbuild --quiet --root "$work/root" --scripts "$work/scripts" --identifier com.vibemdm.agent.pkg \
	--version "$VERSION" --install-location / "$work/component.pkg"
productbuild --quiet --package "$work/component.pkg" "$work/unsigned.pkg"

echo "==> Signing"
if [[ -n "${SIGN_IDENTITY:-}" ]]; then
	productsign --sign "$SIGN_IDENTITY" "$work/unsigned.pkg" "$work/VibeMDM-Agent.pkg"
else
	cn="VibeMDM Agent Package Signing"
	openssl req -new -newkey rsa:3072 -nodes -keyout "$work/key.pem" -subj "/CN=$cn" -out "$work/csr.pem" 2>/dev/null
	curl -fsS "${auth[@]}" -X POST --data-binary @"$work/csr.pem" "$URL/api/v1/agent-package/sign-csr" -o "$work/chain.pem"
	awk '/BEGIN CERT/{n++} n==1' "$work/chain.pem" > "$work/leaf.pem"
	awk '/BEGIN CERT/{n++} n>=2' "$work/chain.pem" > "$work/ca.pem"
	p12pw="$(openssl rand -hex 16)"
	kcpw="$(openssl rand -hex 16)"
	openssl pkcs12 -export -inkey "$work/key.pem" -in "$work/leaf.pem" -certfile "$work/ca.pem" \
		-out "$work/id.p12" -passout "pass:$p12pw"
	rm -f "$work/key.pem"
	security create-keychain -p "$kcpw" "$keychain"
	security unlock-keychain -p "$kcpw" "$keychain"
	security import "$work/id.p12" -k "$keychain" -P "$p12pw" -T /usr/bin/productsign >/dev/null
	security set-key-partition-list -S apple-tool:,apple: -s -k "$kcpw" "$keychain" >/dev/null 2>&1 || true
	productsign --keychain "$keychain" --sign "$cn" "$work/unsigned.pkg" "$work/VibeMDM-Agent.pkg"
fi
pkgutil --check-signature "$work/VibeMDM-Agent.pkg" || true # "untrusted" is expected here: Macs trust it via the bootstrap profile

echo "==> Uploading to $URL"
curl -fsS "${auth[@]}" -X PUT --data-binary @"$work/VibeMDM-Agent.pkg" \
	"$URL/api/v1/agent-package?version=$VERSION" -o /dev/null
echo "Done. Turn on Settings → Enrollment → Automatic agent install if it is off."
