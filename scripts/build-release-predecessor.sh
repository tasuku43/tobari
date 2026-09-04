#!/usr/bin/env bash
set -Eeuo pipefail
cd "$(dirname "$0")/.."

destination=${1:-}
[[ -n $destination && $destination == /* ]] || {
	echo "release predecessor: an absolute output path is required" >&2
	exit 2
}

candidate_revision=$(git rev-parse --verify HEAD)
predecessor_tag=${TOBARI_UPGRADE_PREDECESSOR_TAG:-}
if [[ -z $predecessor_tag ]]; then
	while IFS= read -r tag; do
		[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+-dev\.[0-9]+$ ]] || continue
		[[ $(git cat-file -t "$tag") == tag ]] || continue
		tag_revision=$(git rev-list -n 1 "$tag")
		[[ $tag_revision != "$candidate_revision" ]] || continue
		git merge-base --is-ancestor "$tag_revision" "$candidate_revision" || continue
		predecessor_tag=$tag
		break
	done < <(git tag --merged "$candidate_revision" --sort=-version:refname)
fi
[[ -n $predecessor_tag ]] || {
	echo "release predecessor: no prior annotated development tag is reachable from HEAD" >&2
	exit 1
}
[[ $predecessor_tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+-dev\.[0-9]+$ ]] || {
	echo "release predecessor: tag is not a development SemVer: $predecessor_tag" >&2
	exit 1
}
[[ $(git cat-file -t "$predecessor_tag") == tag ]] || {
	echo "release predecessor: tag is not annotated: $predecessor_tag" >&2
	exit 1
}
predecessor_revision=$(git rev-list -n 1 "$predecessor_tag")
[[ $predecessor_revision != "$candidate_revision" ]] || {
	echo "release predecessor: tag resolves to the candidate revision" >&2
	exit 1
}
git merge-base --is-ancestor "$predecessor_revision" "$candidate_revision" || {
	echo "release predecessor: tag is not an ancestor of the candidate" >&2
	exit 1
}

fixture_root=$(mktemp -d "$PWD/.tobari-release-predecessor.XXXXXX")
cleanup() {
	local status=$?
	trap - EXIT
	rm -rf -- "$fixture_root"
	exit "$status"
}
trap cleanup EXIT

source_root=$fixture_root/source
dist_root=$fixture_root/dist
extract_root=$fixture_root/extract
mkdir -p "$source_root" "$dist_root" "$extract_root" "$(dirname "$destination")"
git archive --format=tar "$predecessor_tag" | tar -xf - -C "$source_root"

goos=$(go env GOOS)
goarch=$(go env GOARCH)
case $goos in
	linux | darwin) ;;
	*) echo "release predecessor: unsupported integration host $goos" >&2; exit 1 ;;
esac
(cd "$source_root" && ./scripts/package-release.sh \
	"$predecessor_tag" "$predecessor_revision" "$goos" "$goarch" "$dist_root")
archive=$dist_root/tobari_${predecessor_tag}_${goos}_${goarch}.tar.gz
tar -xzf "$archive" -C "$extract_root"
install -m 0755 "$extract_root/tobari" "$destination"

identity=$($destination version --format json)
PREDECESSOR_IDENTITY=$identity python3 - "$predecessor_tag" "$predecessor_revision" <<'PY'
import json
import os
import sys

identity = json.loads(os.environ["PREDECESSOR_IDENTITY"])["build_identity"]
if identity != {
    **identity,
    "version": sys.argv[1][1:],
    "commit": sys.argv[2],
    "resolver_channel": "embedded",
    "capability_surface": "release",
    "compatible": True,
}:
    raise SystemExit(f"unexpected predecessor build identity: {identity!r}")
PY
printf 'release predecessor: built %s (%s) for %s/%s\n' \
	"$predecessor_tag" "$predecessor_revision" "$goos" "$goarch"
