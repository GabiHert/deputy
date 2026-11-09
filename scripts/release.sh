#!/usr/bin/env bash
# Builds and publishes a Positron release from this Mac. GitHub Actions is not used.
# Usage: make release              next patch version after the newest tag (v0.2.1 → v0.2.2)
#        make release V=v0.3.0     a version you choose
#
# Steps, stopping at the first failure:
#   1. Checks: on main, nothing uncommitted, in step with origin, the tag and release are new,
#      gh is signed in as GabiHert.
#   2. Tests: go vet and go test, tsc and vitest.
#   3. Build: scripts/build-release.sh (zip + SHA256SUMS; signing from the environment).
#   4. Tag, push main and the tag, then create the GitHub Release with both files.
# A failure before step 4 leaves no tag, push or release behind.
set -euo pipefail
cd "$(dirname "$0")/.."
REPO=GabiHert/positron
ACCOUNT=GabiHert

log() { echo "release: $*"; }
die() {
  echo "release: $*" >&2
  exit 1
}

export GH_TOKEN
GH_TOKEN="$(gh auth token -u "$ACCOUNT" 2>/dev/null)" || die "gh is not signed in as $ACCOUNT. Run: gh auth login (as $ACCOUNT)"

git fetch -q --tags origin main
latest="$(git tag -l 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1)"
tag="${1:-}"
if [ -z "$tag" ]; then
  [ -n "$latest" ] || die "there is no vX.Y.Z tag yet; run: make release V=v0.1.0"
  IFS=. read -r major minor patch <<<"${latest#v}"
  tag="v$major.$minor.$((patch + 1))"
fi
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || die "'$tag' is not a vX.Y.Z version"

log "releasing $tag (newest tag: ${latest:-none})"
[ "$(git branch --show-current)" = main ] || die "switch to main first"
[ -z "$(git status --porcelain)" ] || die "commit or stash your changes first:
$(git status --short)"
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] ||
  git merge-base --is-ancestor origin/main HEAD || die "main is behind origin/main; pull first"
! git rev-parse -q --verify "refs/tags/$tag" >/dev/null || die "tag $tag already exists"
! gh release view "$tag" --repo "$REPO" >/dev/null 2>&1 || die "release $tag already exists on GitHub"

log "testing"
go vet ./internal/...
go test ./internal/...
(cd frontend && npx tsc --noEmit && npx vitest run)

bash scripts/build-release.sh "$tag"

log "tagging and pushing $tag"
git tag -a "$tag" -m "Positron $tag"
git push -q origin main
git push -q origin "$tag"

notes="Built and signed on the developer's Mac with make release."
if ! codesign -dvv build/bin/Positron.app 2>&1 | grep -q '^TeamIdentifier=[A-Z0-9]'; then
  notes="$notes Signed ad hoc and not notarized."
fi
prerelease=()
case "$tag" in *-*) prerelease=(--prerelease) ;; esac
gh release create "$tag" build/release/* --repo "$REPO" --title "Positron $tag" --verify-tag \
  --generate-notes --notes "$notes" ${prerelease[@]+"${prerelease[@]}"}
log "published https://github.com/$REPO/releases/tag/$tag"
