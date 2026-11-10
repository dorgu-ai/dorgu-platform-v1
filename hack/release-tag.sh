#!/usr/bin/env bash
# Tag a commit that carries the built SPA, so a Go module consumer (the dorgu CLI)
# fetches a module whose go:embed has something to embed.
#
# main never contains build output. The release commit is a child of the current
# HEAD, made on a detached HEAD, so it is reachable only through its tag. Nothing
# is pushed: the last line printed is the command to run once you have looked.
#
# Usage: hack/release-tag.sh vX.Y.Z

set -euo pipefail

die() { echo "release-tag: $*" >&2; exit 1; }

tag="${1:-}"
[[ -n "$tag" ]] || die "usage: hack/release-tag.sh vX.Y.Z"
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || die "'$tag' is not a semantic version tag like v0.1.0"

cd "$(git rev-parse --show-toplevel)"

git diff --quiet && git diff --cached --quiet || die "the working tree has uncommitted changes"
[[ -z "$(git ls-files --others --exclude-standard)" ]] || die "the working tree has untracked files"
git rev-parse -q --verify "refs/tags/$tag" >/dev/null && die "tag $tag already exists"

origin_head="$(git rev-parse HEAD)"
start_ref="$(git symbolic-ref -q --short HEAD || echo "$origin_head")"

make web
[[ -f internal/webui/dist/index.html ]] || die "the SPA build produced no internal/webui/dist/index.html"

# Everything below happens on a detached HEAD; always return to where we were.
trap 'git switch -q "$start_ref" 2>/dev/null || git checkout -q "$start_ref"' EXIT
git switch -q --detach

# dist is gitignored on main, so it has to be forced in.
git add -f internal/webui/dist
git -c user.useConfigOnly=false commit -q -m "chore: release $tag with the built SPA"
git tag -a "$tag" -m "$tag"

# Prove the tagged tree embeds a real SPA, as a consumer's build would see it.
check="$(mktemp -d)"
trap 'rm -rf "$check"; git switch -q "$start_ref" 2>/dev/null || git checkout -q "$start_ref"' EXIT
git archive "$tag" internal/webui | tar -x -C "$check"
[[ -f "$check/internal/webui/dist/index.html" ]] || die "the tag does not contain internal/webui/dist/index.html"
asset_count="$(find "$check/internal/webui/dist" -type f | wc -l | tr -d ' ')"

echo
echo "Tagged $tag at $(git rev-parse --short "$tag") (parent $(git rev-parse --short "$origin_head")), with $asset_count embedded files."
echo "main is unchanged. To publish:"
echo "  git push origin refs/tags/$tag"
