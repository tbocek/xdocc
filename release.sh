#!/usr/bin/env bash
#
# Cut a release: check, test, build for every platform, tag, upload to GitHub.
#
#   ./release.sh v0.1.0          make the release
#   ./release.sh v0.1.0 --dry-run  build everything, but do not tag or upload
#
# Needs: go, git, and the GitHub CLI (gh) logged in.

set -euo pipefail

readonly REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly DIST="${REPO_DIR}/dist"
readonly BINARY="xdocc"
readonly MAIN="./cmd/xdocc"

# platform list: GOOS/GOARCH
readonly PLATFORMS=(
	linux/amd64
	linux/arm64
	linux/arm
	darwin/amd64
	darwin/arm64
	freebsd/amd64
	windows/amd64
)

die() { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }
step() { printf '\033[1m==>\033[0m %s\n' "$*"; }

VERSION="${1:-}"
DRY_RUN="${2:-}"
[[ -n "${VERSION}" ]] || die "usage: $0 vX.Y.Z [--dry-run]"
[[ "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] ||
	die "version must look like v1.2.3 or v1.2.3-rc1, got '${VERSION}'"
[[ -z "${DRY_RUN}" || "${DRY_RUN}" == "--dry-run" ]] || die "unknown argument '${DRY_RUN}'"
readonly VERSION DRY_RUN

cd "${REPO_DIR}"

step "checking the working tree"
command -v go >/dev/null || die "go is not installed"
command -v git >/dev/null || die "git is not installed"
if [[ -z "${DRY_RUN}" ]]; then
	command -v gh >/dev/null || die "the GitHub CLI (gh) is not installed: https://cli.github.com"
	gh auth status >/dev/null 2>&1 || die "gh is not logged in, run: gh auth login"
fi
# a dry run only builds, so a dirty tree is a warning there and a stop here
complain() {
	if [[ -n "${DRY_RUN}" ]]; then
		printf '\033[33mwarning:\033[0m %s\n' "$*" >&2
	else
		die "$*"
	fi
}
if [[ -n "$(git status --porcelain)" ]]; then
	complain "the working tree is dirty, commit or stash first"
fi
if [[ -z "${DRY_RUN}" ]]; then
	git fetch --tags --quiet origin
fi
if git rev-parse -q --verify "refs/tags/${VERSION}" >/dev/null; then
	complain "tag ${VERSION} already exists"
fi
BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [[ "${BRANCH}" != "main" && -z "${DRY_RUN}" ]]; then
	printf 'you are on branch %s, not main. continue? [y/N] ' "${BRANCH}"
	read -r reply && [[ "${reply}" == [yY] ]] || die "aborted"
fi
if [[ -z "${DRY_RUN}" && -n "$(git log --oneline "origin/${BRANCH}..${BRANCH}" 2>/dev/null)" ]]; then
	die "there are unpushed commits, push them first"
fi

step "gofmt"
UNFORMATTED="$(gofmt -l . | grep -v '^old/' || true)"
[[ -z "${UNFORMATTED}" ]] || die "not gofmt'ed:"$'\n'"${UNFORMATTED}"

step "go vet"
go vet ./...

step "go test"
go test ./...

step "building ${VERSION}"
rm -rf "${DIST}"
mkdir -p "${DIST}"
# -s -w strip the debug info, trimpath keeps build paths out of the binary
LDFLAGS="-s -w -X main.version=${VERSION}"
for platform in "${PLATFORMS[@]}"; do
	GOOS="${platform%/*}"
	GOARCH="${platform#*/}"
	name="${BINARY}_${VERSION}_${GOOS}_${GOARCH}"
	exe="${BINARY}"
	if [[ "${GOOS}" == "windows" ]]; then exe="${BINARY}.exe"; fi

	work="${DIST}/${name}"
	mkdir -p "${work}"
	printf '    %s\n' "${platform}"
	CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" \
		go build -trimpath -ldflags "${LDFLAGS}" -o "${work}/${exe}" "${MAIN}"
	cp README.md "${work}/"
	if [[ -f LICENSE ]]; then cp LICENSE "${work}/"; fi
	if [[ -d contrib ]]; then cp -r contrib "${work}/"; fi

	if [[ "${GOOS}" == "windows" ]]; then
		command -v zip >/dev/null || die "zip is not installed, needed for the windows archive"
		(cd "${DIST}" && zip -qr "${name}.zip" "${name}")
	else
		tar -czf "${DIST}/${name}.tar.gz" -C "${DIST}" "${name}"
	fi
	rm -rf "${work}"
done

step "checksums"
(cd "${DIST}" && sha256sum ./*.tar.gz ./*.zip > "${BINARY}_${VERSION}_checksums.txt")
cat "${DIST}/${BINARY}_${VERSION}_checksums.txt"

# release notes: every commit since the previous tag
PREVIOUS="$(git describe --tags --abbrev=0 2>/dev/null || true)"
NOTES_FILE="${DIST}/notes.md"
{
	if [[ -n "${PREVIOUS}" ]]; then
		echo "## Changes since ${PREVIOUS}"
		echo
		git log --no-merges --pretty='- %s' "${PREVIOUS}..HEAD"
	else
		echo "## ${VERSION}"
		echo
		echo "First release."
	fi
	echo
	echo '## Install'
	echo
	echo '```'
	echo "tar xzf ${BINARY}_${VERSION}_linux_amd64.tar.gz"
	echo "sudo install -m 755 ${BINARY}_${VERSION}_linux_amd64/${BINARY} /usr/local/bin/${BINARY}"
	echo '```'
	echo
	echo 'Or with Go:'
	echo
	echo '```'
	echo "go install github.com/tbocek/xdocc/cmd/xdocc@${VERSION}"
	echo '```'
} > "${NOTES_FILE}"

if [[ -n "${DRY_RUN}" ]]; then
	step "dry run: not tagging, not uploading"
	echo "artifacts are in ${DIST}"
	exit 0
fi

echo
step "about to publish ${VERSION} to $(git remote get-url origin)"
ls -1 "${DIST}"/*.tar.gz "${DIST}"/*.zip "${DIST}"/*checksums.txt | sed 's|.*/|    |'
printf 'this tags, pushes and creates a public GitHub release. continue? [y/N] '
read -r reply && [[ "${reply}" == [yY] ]] || die "aborted, nothing was pushed"

step "tagging"
git tag -a "${VERSION}" -m "${BINARY} ${VERSION}"
git push origin "${VERSION}"

step "creating the GitHub release"
PRERELEASE=()
if [[ "${VERSION}" == *-* ]]; then PRERELEASE=(--prerelease); fi
gh release create "${VERSION}" \
	--title "${BINARY} ${VERSION}" \
	--notes-file "${NOTES_FILE}" \
	"${PRERELEASE[@]}" \
	"${DIST}"/*.tar.gz "${DIST}"/*.zip "${DIST}/${BINARY}_${VERSION}_checksums.txt"

step "done: $(gh release view "${VERSION}" --json url --jq .url)"
