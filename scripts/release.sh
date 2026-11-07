#!/usr/bin/env bash
# Builds a Deputy release into build/release: Deputy-<tag>-darwin-arm64.zip and SHA256SUMS.
# Usage: scripts/release.sh v1.2.3
# CI runs it on each v* tag (.github/workflows/release.yml); it also runs on your Mac.
#
# Signing, from the environment:
#   DEPUTY_SIGN_IDENTITY   "Developer ID Application: Name (TEAMID)". Unset: ad hoc.
#   NOTARY_KEY_PATH, NOTARY_KEY_ID, NOTARY_ISSUER
#                          An App Store Connect API key (.p8) for notarytool. With these and a
#                          Developer ID, the app is notarized and the ticket stapled.
set -euo pipefail
cd "$(dirname "$0")/.."
tag="${1:?usage: scripts/release.sh vX.Y.Z}"
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || { echo "release: '$tag' is not a vX.Y.Z tag" >&2; exit 1; }
version="${tag#v}"
APP=build/bin/Deputy.app
OUT=build/release
zip_name="Deputy-$tag-darwin-arm64.zip"

log() { echo "release: $*"; }

rm -rf "$OUT"
mkdir -p "$OUT"

log "building $tag"
DEPUTY_SIGN_IDENTITY="" make build VERSION="$tag"

# The bundle's version is the tag, so Finder and macOS show the release.
plutil -replace CFBundleShortVersionString -string "$version" "$APP/Contents/Info.plist"
plutil -replace CFBundleVersion -string "$version" "$APP/Contents/Info.plist"

if [ -n "${DEPUTY_SIGN_IDENTITY:-}" ]; then
  bash scripts/sign-bundle.sh "$APP"
else
  log "DEPUTY_SIGN_IDENTITY is not set: this release is signed ad hoc and is not notarized"
  for lib in "$APP"/Contents/Frameworks/*.dylib; do
    [ -e "$lib" ] && codesign -f -s - "$lib"
  done
  codesign -f -s - "$APP"
fi
bash scripts/check-bundle.sh "$APP"

# ditto keeps the bundle's symlinks, permissions and signature; --keepParent puts Deputy.app at the
# top of the zip, which is where the updater looks.
ditto -c -k --keepParent "$APP" "$OUT/$zip_name"

if [ -n "${DEPUTY_SIGN_IDENTITY:-}" ] && [ -n "${NOTARY_KEY_PATH:-}" ]; then
  log "notarizing (this takes a few minutes)"
  xcrun notarytool submit "$OUT/$zip_name" --key "$NOTARY_KEY_PATH" --key-id "$NOTARY_KEY_ID" \
    --issuer "$NOTARY_ISSUER" --wait --timeout 30m
  xcrun stapler staple "$APP"
  xcrun stapler validate "$APP"
  spctl --assess --type execute --verbose "$APP"
  # Zip again so the download carries the stapled ticket and opens offline.
  rm -f "$OUT/$zip_name"
  ditto -c -k --keepParent "$APP" "$OUT/$zip_name"
elif [ -n "${DEPUTY_SIGN_IDENTITY:-}" ]; then
  log "NOTARY_KEY_PATH is not set: signed with a Developer ID but not notarized; Gatekeeper will refuse it"
fi

(cd "$OUT" && shasum -a 256 "$zip_name" > SHA256SUMS)
log "done:"
ls -l "$OUT"
cat "$OUT/SHA256SUMS"
