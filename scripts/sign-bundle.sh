#!/usr/bin/env bash
# Signs the built Positron.app inside-out.
# With POSITRON_SIGN_IDENTITY set (a "Developer ID Application: …" name or its SHA-1), it signs with
# that identity, the hardened runtime, a secure timestamp and build/darwin/entitlements.plist, which
# notarization needs. Otherwise it signs with "Positron Local Signing" when the keychain has it, else
# ad hoc.
set -euo pipefail
cd "$(dirname "$0")/.."
APP="${1:-build/bin/Positron.app}"
C="$APP/Contents"
opts=()
if [ -n "${POSITRON_SIGN_IDENTITY:-}" ]; then
  id="$POSITRON_SIGN_IDENTITY"
  opts=(--options runtime --timestamp)
  echo "sign-bundle: signing with $id (hardened runtime)"
else
  id="$(bash scripts/signing-identity.sh)"
  if [ "$id" = "-" ]; then
    echo 'sign-bundle: no "Positron Local Signing" certificate in your keychain; signing ad hoc. macOS may ask again for permissions after each build. Run bash scripts/create-signing-identity.sh once to stop that.'
  else
    echo 'sign-bundle: signing with "Positron Local Signing"'
  fi
fi
for lib in "$C"/Frameworks/*.dylib; do
  [ -e "$lib" ] || continue
  codesign -f -s "$id" ${opts[@]+"${opts[@]}"} "$lib"
done
if [ -n "${POSITRON_SIGN_IDENTITY:-}" ]; then
  codesign -f -s "$id" "${opts[@]}" --entitlements build/darwin/entitlements.plist "$APP"
else
  codesign -f -s "$id" "$APP"
fi
