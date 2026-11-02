#!/usr/bin/env bash
# Prepare a Rowan release on the local main branch.
#
# Usage: scripts/release.sh vX.Y.Z
#
# The script checks the tag, the git tree and CHANGELOG.md, and runs the Go
# and frontend tests. Then it moves the Unreleased changes into a dated
# section, sets the version in frontend/package.json and wails.json, commits
# "Release vX.Y.Z" and creates an annotated tag. It does not push.
#
# To include the PostgreSQL and MySQL engine tests, set ROWAN_TEST_PG_PORT
# and ROWAN_TEST_MYSQL_PORT. README.md shows how to start the servers.

set -euo pipefail

die() {
	printf 'release: %s\n' "$*" >&2
	exit 1
}

step() {
	printf '\n==> %s\n' "$*"
}

if [[ $# -ne 1 ]]; then
	echo "usage: scripts/release.sh vX.Y.Z" >&2
	echo "example: scripts/release.sh v1.2.3" >&2
	exit 2
fi

tag=$1
semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
if [[ ! $tag =~ $semver ]]; then
	die "the tag $tag does not match vX.Y.Z or vX.Y.Z-prerelease, such as v1.2.3 or v1.3.0-rc.1"
fi
version=${tag#v}
# Windows and macOS version fields accept numbers only. wails.json gets the
# X.Y.Z part of a prerelease version.
core=${version%%-*}

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

for cmd in git go node npm awk; do
	command -v "$cmd" >/dev/null 2>&1 || die "$cmd is not installed"
done

step "Check the git tree"
git rev-parse --is-inside-work-tree >/dev/null 2>&1 || die "$root is not a git repository"
branch=$(git symbolic-ref --quiet --short HEAD || true)
if [[ $branch != main ]]; then
	die "check out main first. The current branch is ${branch:-a detached HEAD}"
fi
if [[ -n $(git status --porcelain) ]]; then
	die "the working tree has changes. Commit or stash them first"
fi
if git rev-parse --quiet --verify "refs/tags/$tag" >/dev/null; then
	die "the tag $tag exists already"
fi

step "Check CHANGELOG.md"
[[ -f CHANGELOG.md ]] || die "CHANGELOG.md is missing"
headers=$(awk '$0 == "## [Unreleased]" { n++ } END { print n + 0 }' CHANGELOG.md)
if [[ $headers -ne 1 ]]; then
	die "CHANGELOG.md must have exactly one '## [Unreleased]' line. It has $headers"
fi
if awk -v v="$version" 'index($0, "## [" v "]") == 1 { found = 1 } END { exit !found }' CHANGELOG.md; then
	die "CHANGELOG.md has a section for $version already"
fi
# Count the lines in the Unreleased section that are not blank and not a
# "###" heading. The section ends at the next "## " heading or at the link
# references.
entries=$(awk '
	$0 == "## [Unreleased]" { inside = 1; next }
	inside && (/^## / || /^\[[^]]+\]: /) { exit }
	inside && /[^[:space:]]/ && !/^### / { n++ }
	END { print n + 0 }
' CHANGELOG.md)
if [[ $entries -eq 0 ]]; then
	die "the Unreleased section in CHANGELOG.md is empty. Add the changes for $tag first"
fi

step "Frontend: npm ci, npm test, npm run check, npm run build"
(cd frontend && npm ci && npm test && npm run check && npm run build)

step "Go: go vet and go test"
go_tags=()
if [[ $(uname -s) == Linux ]]; then
	go_tags=(-tags webkit2_41)
fi
# The tests use a temporary config directory and file credentials, so they
# do not touch your Rowan config or your keychain.
test_config=$(mktemp -d)
trap 'rm -rf "$test_config"' EXIT
export ROWAN_CONFIG_DIR=$test_config ROWAN_CREDENTIALS=file
# ${go_tags[@]+...} keeps bash 3.2 on macOS from failing on an empty array.
go vet ${go_tags[@]+"${go_tags[@]}"} ./...
go test ${go_tags[@]+"${go_tags[@]}"} ./...
unset ROWAN_CONFIG_DIR ROWAN_CREDENTIALS

changed=(CHANGELOG.md frontend/package.json frontend/package-lock.json wails.json)
trap 'echo "release: the release stopped. To undo the file changes, run: git checkout -- ${changed[*]}" >&2' ERR

step "Update CHANGELOG.md, frontend/package.json and wails.json"
today=$(date +%Y-%m-%d)
awk -v header="## [$version] - $today" '
	{ print }
	$0 == "## [Unreleased]" { print ""; print header }
' CHANGELOG.md >CHANGELOG.md.tmp
mv CHANGELOG.md.tmp CHANGELOG.md

(cd frontend && npm version "$version" --no-git-tag-version --allow-same-version >/dev/null)

node -e '
const fs = require("fs")
const file = "wails.json"
const project = JSON.parse(fs.readFileSync(file, "utf8"))
project.info = { ...project.info, productVersion: process.argv[1] }
fs.writeFileSync(file, JSON.stringify(project, null, 2) + "\n")
' "$core"

step "Commit and tag"
git add -- "${changed[@]}"
git commit --quiet -m "Release $tag"
trap 'echo "release: the tag step failed. To undo the commit, run: git reset --hard HEAD~1" >&2' ERR
git tag -a "$tag" -m "Release $tag"
trap - ERR

remote=$(git config --get branch.main.remote || echo origin)
echo
echo "The commit \"Release $tag\" and the tag $tag are ready."
if [[ $version == *-* ]]; then
	echo "The tag has a \"-\", so the GitHub release is a prerelease."
fi
echo "To check them, run: git show $tag"
echo "To start the release workflow, run:"
echo
echo "  git push --atomic $remote main $tag"
