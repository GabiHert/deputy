#!/usr/bin/env bash
# Bundle check: the built Deputy.app carries its helper and speech libraries and is signed ad hoc or with Deputy Local Signing.
set -uo pipefail
cd "$(dirname "$0")/.."

APP="${1:-build/bin/Deputy.app}"
C="$APP/Contents"

fail() {
  echo "check-bundle: FAIL $*"
  exit 1
}

[ -x "$C/MacOS/Deputy" ] || fail "missing Contents/MacOS/Deputy"
[ -f "$C/Frameworks/libsherpa-onnx-c-api.dylib" ] || fail "missing Contents/Frameworks/libsherpa-onnx-c-api.dylib"
compgen -G "$C/Frameworks/libonnxruntime*.dylib" >/dev/null || fail "missing Contents/Frameworks/libonnxruntime*.dylib"

rpaths="$(otool -l "$C/MacOS/Deputy" | awk '/LC_RPATH/{grab=2; next} grab && /path /{print $2; grab=0}')"
case "$rpaths" in
  *sherpa-onnx-go-macos*) fail "LC_RPATH into the Go module cache" ;;
esac
case "$rpaths" in
  *"@executable_path/../Frameworks"*) ;;
  *) fail "LC_RPATH @executable_path/../Frameworks not present" ;;
esac

codesign --verify --deep --strict "$APP" 2>/dev/null || fail "codesign --verify --deep --strict"

id="$(plutil -extract CFBundleIdentifier raw "$C/Info.plist" 2>/dev/null)"
[ "$id" = "tech.bhlabs.deputy" ] || fail "CFBundleIdentifier is '$id'"


# kind_of <path>: adhoc, local (Deputy Local Signing) or other.
kind_of() {
  local info
  info="$(codesign -dv --verbose=2 "$1" 2>&1)"
  if grep -qx 'Signature=adhoc' <<<"$info"; then
    echo adhoc
  elif grep -qx 'Authority=Deputy Local Signing' <<<"$info"; then
    echo local
  else
    echo other
  fi
}
want="$(kind_of "$APP")"
[ "$want" != other ] || fail "Deputy.app is signed neither ad hoc nor with Deputy Local Signing"
for f in "$C"/Frameworks/*.dylib; do
  [ -e "$f" ] || continue
  [ "$(kind_of "$f")" = "$want" ] || fail "${f#"$C/"} is signed differently from Deputy.app"
done
if [ "$want" = adhoc ]; then
  echo "check-bundle: OK (signed ad hoc)"
else
  echo "check-bundle: OK (signed with Deputy Local Signing)"
fi
