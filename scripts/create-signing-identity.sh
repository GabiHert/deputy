#!/usr/bin/env bash
# Creates the self-signed code-signing identity "Deputy Local Signing" once, so every build of
# Deputy keeps the same signature and macOS keeps its microphone and screen recording grants.
set -euo pipefail
NAME="Deputy Local Signing"
KEYCHAIN="${DEPUTY_KEYCHAIN:-$HOME/Library/Keychains/login.keychain-db}"
OPENSSL="${DEPUTY_OPENSSL:-/usr/bin/openssl}"
UNTRUSTED='The certificate "Deputy Local Signing" is in your keychain, but macOS does not trust it for code signing. Open Keychain Access, double-click "Deputy Local Signing", open Trust, and set Code Signing to Always Trust. Then run make build.'

# has_identity [-v]: reads all of the listing before matching, so grep never closes the pipe early.
has_identity() {
  local ids
  ids="$(security find-identity "$@" -p codesigning 2>/dev/null || true)"
  grep -F "\"$NAME\"" <<<"$ids" >/dev/null
}

if ! command -v security >/dev/null 2>&1; then
  echo "This script needs macOS."
  exit 1
fi

if has_identity -v; then
  echo 'The code-signing identity "Deputy Local Signing" already exists. Nothing to do.'
  exit 0
fi

if has_identity; then
  echo "$UNTRUSTED"
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
echo "Creating the code-signing identity \"Deputy Local Signing\" in $KEYCHAIN."

cat >"$tmp/req.cnf" <<'CNF'
[req]
distinguished_name=dn
prompt=no
x509_extensions=ext
[dn]
CN=Deputy Local Signing
[ext]
basicConstraints=critical,CA:false
keyUsage=critical,digitalSignature
extendedKeyUsage=critical,codeSigning
subjectKeyIdentifier=hash
CNF
"$OPENSSL" req -x509 -newkey rsa:2048 -sha256 -nodes -days 3650 -config "$tmp/req.cnf" -keyout "$tmp/key.pem" -out "$tmp/cert.pem" 2>/dev/null

legacy=""
if "$OPENSSL" version | grep -q '^OpenSSL 3'; then
  legacy="-legacy"
fi
DEPUTY_P12_PASS="$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 24 || true)"
export DEPUTY_P12_PASS
# shellcheck disable=SC2086
"$OPENSSL" pkcs12 -export $legacy -inkey "$tmp/key.pem" -in "$tmp/cert.pem" -name "$NAME" -out "$tmp/id.p12" -passout env:DEPUTY_P12_PASS

security import "$tmp/id.p12" -k "$KEYCHAIN" -P "$DEPUTY_P12_PASS" -T /usr/bin/codesign

echo "macOS now asks for your login password to trust the certificate for code signing."
if ! security add-trusted-cert -r trustRoot -p codeSign -k "$KEYCHAIN" "$tmp/cert.pem"; then
  echo "$UNTRUSTED"
  exit 1
fi

if ! has_identity -v; then
  echo "$UNTRUSTED"
  exit 1
fi
echo 'Created the code-signing identity "Deputy Local Signing". Run make build: macOS asks for the microphone and Screen & System Audio Recording permissions once more, then keeps them across builds and updates.'
exit 0
