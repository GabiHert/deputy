#!/usr/bin/env bash
# Signs the built Deputy.app inside-out.
# With DEPUTY_SIGN_IDENTITY set (a "Developer ID Application: …" name or its SHA-1), it signs with
# that identity, the hardened runtime, a secure timestamp and build/darwin/entitlements.plist, which
# notarization needs. Otherwise it signs with "Deputy Local Signing" when the keychain has it, else
# ad hoc.
set -euo pipefail
cd "$(dirname "$0")/.."
APP="${1:-build/bin/Deputy.app}"
C="$APP/Contents"
opts=()
if [ -n "${DEPUTY_SIGN_IDENTITY:-}" ]; then
  id="$DEPUTY_SIGN_IDENTITY"
  opts=(--options runtime --timestamp)
  echo "sign-bundle: signing with $id (hardened runtime)"
else
  id="$(bash scripts/signing-identity.sh)"
  if [ "$id" = "-" ]; then
    echo 'sign-bundle: no "Deputy Local Signing" certificate in your keychain; signing ad hoc. macOS may ask again for permissions after each build. Run bash scripts/create-signing-identity.sh once to stop that.'
  else
    echo 'sign-bundle: signing with "Deputy Local Signing"'
  fi
fi
for lib in "$C"/Frameworks/*.dylib; do
  [ -e "$lib" ] || continue
  codesign -f -s "$id" ${opts[@]+"${opts[@]}"} "$lib"
done
if [ -n "${DEPUTY_SIGN_IDENTITY:-}" ]; then
  codesign -f -s "$id" "${opts[@]}" --entitlements build/darwin/entitlements.plist "$APP"
else
  codesign -f -s "$id" "$APP"
fi
