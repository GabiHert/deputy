#!/usr/bin/env bash
# Signs the built Deputy.app inside-out with "Deputy Local Signing" when the keychain has it, else ad hoc.
set -euo pipefail
cd "$(dirname "$0")/.."
APP="${1:-build/bin/Deputy.app}"
C="$APP/Contents"
id="$(bash scripts/signing-identity.sh)"
if [ "$id" = "-" ]; then
  echo 'sign-bundle: no "Deputy Local Signing" certificate in your keychain; signing ad hoc. macOS may ask again for permissions after each build. Run bash scripts/create-signing-identity.sh once to stop that.'
else
  echo 'sign-bundle: signing with "Deputy Local Signing"'
fi
for lib in "$C"/Frameworks/*.dylib; do
  [ -e "$lib" ] || continue
  codesign -f -s "$id" "$lib"
done
codesign -f -s "$id" "$APP"
