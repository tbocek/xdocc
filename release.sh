#!/usr/bin/env bash
#
# Cut a release. The build itself happens on GitHub Actions, from the tag: this
# checks the tree, tags it, pushes, and then waits for the workflow to finish so
# you learn here whether the release is good rather than by looking later.
#
#   ./release.sh v0.1.0             tag, push, wait for the build
#   ./release.sh v0.1.0 --dry-run   check and build everything locally, no tag
#   ./release.sh v0.1.0 --package   build and package into dist/, nothing else
#
# --package is what the workflow runs, so the platform list, the archives and
# the release notes are written down once and are the same either way.
#
# Needs: go, git. Waiting also needs curl and jq.

set -euo pipefail

readonly REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly DIST="${REPO_DIR}/dist"
readonly BINARY="xdocc"
readonly MAIN="./cmd/xdocc"
readonly SLUG="tbocek/xdocc"
readonly WORKFLOW="build.yml"
readonly IMAGE="ghcr.io/tbocek/xdocc"

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
# what the workflow attaches: one archive per platform, plus the checksums
readonly EXPECTED=$((${#PLATFORMS[@]} + 1))

# how long to wait for the workflow, in RETRIES cycles of SLEEP seconds
readonly RETRIES=40
readonly SLEEP=15

die() { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }
step() { printf '\033[1m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33mwarning:\033[0m %s\n' "$*" >&2; }

VERSION=""
MODE="release"
for arg in "$@"; do
	case "${arg}" in
	--dry-run) MODE="dry-run" ;;
	--package) MODE="package" ;;
	-h | --help)
		sed -n '2,/^$/p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	-*) die "unknown argument '${arg}'" ;;
	*)
		[[ -z "${VERSION}" ]] || die "two versions given: '${VERSION}' and '${arg}'"
		VERSION="${arg}"
		;;
	esac
done
[[ -n "${VERSION}" ]] || die "usage: $0 vX.Y.Z [--dry-run|--package]"
[[ "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] ||
	die "version must look like v1.2.3 or v1.2.3-rc1, got '${VERSION}'"
readonly VERSION MODE

cd "${REPO_DIR}"

# ---------------------------------------------------------------- build

# package builds every platform into dist/, with checksums and release notes.
# It touches nothing outside dist/ and needs no network, which is what lets the
# workflow and a dry run share it.
package() {
	step "building ${VERSION}"
	rm -rf "${DIST}"
	mkdir -p "${DIST}"
	# -s -w strip the debug info, -trimpath keeps build paths out of the binary
	local ldflags="-s -w -X main.version=${VERSION}"
	local platform goos goarch name exe work
	for platform in "${PLATFORMS[@]}"; do
		goos="${platform%/*}"
		goarch="${platform#*/}"
		name="${BINARY}_${VERSION}_${goos}_${goarch}"
		exe="${BINARY}"
		[[ "${goos}" == "windows" ]] && exe="${BINARY}.exe"

		work="${DIST}/${name}"
		mkdir -p "${work}"
		printf '    %s\n' "${platform}"
		CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
			go build -trimpath -ldflags "${ldflags}" -o "${work}/${exe}" "${MAIN}"
		cp README.md "${work}/"
		[[ -f LICENSE ]] && cp LICENSE "${work}/"
		[[ -d contrib ]] && cp -r contrib "${work}/"

		if [[ "${goos}" == "windows" ]]; then
			command -v zip >/dev/null || die "zip is not installed, needed for the windows archive"
			(cd "${DIST}" && zip -qr "${name}.zip" "${name}")
		else
			tar -czf "${DIST}/${name}.tar.gz" -C "${DIST}" "${name}"
		fi
		rm -rf "${work}"
	done

	step "checksums"
	(cd "${DIST}" && sha256sum ./*.tar.gz ./*.zip >"${BINARY}_${VERSION}_checksums.txt")
	cat "${DIST}/${BINARY}_${VERSION}_checksums.txt"

	notes >"${DIST}/notes.md"
}

# notes writes the release notes: every commit since the previous tag, and how
# to install what this release ships.
notes() {
	# in the workflow the tag exists and describes itself, so the previous one
	# is asked for from the commit before it; locally the tag is not there yet
	local previous
	previous="$(git describe --tags --abbrev=0 "${VERSION}^" 2>/dev/null ||
		git describe --tags --abbrev=0 2>/dev/null || true)"

	if [[ -n "${previous}" && "${previous}" != "${VERSION}" ]]; then
		echo "## Changes since ${previous}"
		echo
		git log --no-merges --pretty='- %s' "${previous}..${VERSION}" 2>/dev/null ||
			git log --no-merges --pretty='- %s' "${previous}..HEAD"
	else
		echo "## ${VERSION}"
		echo
		echo "First release."
	fi
	cat <<-EOF

		## Install

		\`\`\`
		tar xzf ${BINARY}_${VERSION}_linux_amd64.tar.gz
		sudo install -m 755 ${BINARY}_${VERSION}_linux_amd64/${BINARY} /usr/local/bin/${BINARY}
		\`\`\`

		Or with Go:

		\`\`\`
		go install github.com/${SLUG}/cmd/${BINARY}@${VERSION}
		\`\`\`

		Or as a container:

		\`\`\`
		docker run --rm -v ./site:/srv/site:ro -v ./www:/srv/www ${IMAGE}:${VERSION}
		\`\`\`
	EOF
}

if [[ "${MODE}" == "package" ]]; then
	package
	exit 0
fi

# ---------------------------------------------------------------- checks

step "checking the working tree"
command -v go >/dev/null || die "go is not installed"
command -v git >/dev/null || die "git is not installed"

# a dry run only builds, so a dirty tree is a warning there and a stop here
complain() {
	if [[ "${MODE}" == "dry-run" ]]; then warn "$*"; else die "$*"; fi
}
[[ -z "$(git status --porcelain)" ]] || complain "the working tree is dirty, commit or stash first"
if [[ "${MODE}" == "release" ]]; then
	command -v curl >/dev/null || die "curl is not installed, needed to wait for the build"
	command -v jq >/dev/null || die "jq is not installed, needed to wait for the build"
	git fetch --tags --quiet origin
fi
if git rev-parse -q --verify "refs/tags/${VERSION}" >/dev/null; then
	complain "tag ${VERSION} already exists"
fi
BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [[ "${BRANCH}" != "main" && "${MODE}" == "release" ]]; then
	printf 'you are on branch %s, not main. continue? [y/N] ' "${BRANCH}"
	read -r reply && [[ "${reply}" == [yY] ]] || die "aborted"
fi
if [[ "${MODE}" == "release" && -n "$(git log --oneline "origin/${BRANCH}..${BRANCH}" 2>/dev/null)" ]]; then
	die "there are unpushed commits, push them first"
fi

step "gofmt"
UNFORMATTED="$(gofmt -l . | grep -v '^old/' || true)"
[[ -z "${UNFORMATTED}" ]] || die "not gofmt'ed:"$'\n'"${UNFORMATTED}"

step "go vet"
go vet ./...

step "go test"
go test ./...

# The workflow builds the image too, and a Dockerfile that does not build would
# only be found out after the tag is pushed. Building it here first costs a
# minute and keeps a broken image from becoming a released one.
if [[ -f Dockerfile ]] && command -v docker >/dev/null && docker buildx version >/dev/null 2>&1; then
	step "docker image (both architectures, not pushed)"
	docker buildx build --platform linux/amd64,linux/arm64 \
		--build-arg "VERSION=${VERSION}" --output type=cacheonly . ||
		die "the image does not build"
elif [[ -f Dockerfile ]]; then
	warn "docker buildx is not here, so the image is only checked by the workflow"
fi

if [[ "${MODE}" == "dry-run" ]]; then
	package
	step "dry run: not tagging, not pushing"
	echo "artifacts are in ${DIST}"
	exit 0
fi

# ---------------------------------------------------------------- publish

echo
step "about to release ${VERSION} from $(git remote get-url origin)"
echo "    the tag is pushed, and the workflow builds ${#PLATFORMS[@]} platforms,"
echo "    attaches them to the release and pushes ${IMAGE}:${VERSION}"
printf 'continue? [y/N] '
read -r reply && [[ "${reply}" == [yY] ]] || die "aborted, nothing was pushed"

step "tagging"
git tag -a "${VERSION}" -m "${BINARY} ${VERSION}"
git push origin "${VERSION}"

# ---------------------------------------------------------------- wait

# Poll with as few API calls as possible: the unauthenticated GitHub API allows
# only 60 requests/hr per IP, and polling both endpoints frequently exhausts it.
# So one call every ${SLEEP}s for the run status, and only once the build
# succeeds a single call to confirm the assets. On a rate limit we abort loudly.
SHA="$(git rev-list -n1 "${VERSION}")"
RUNS_URL="https://api.github.com/repos/${SLUG}/actions/workflows/${WORKFLOW}/runs?head_sha=${SHA}"
REL_URL="https://api.github.com/repos/${SLUG}/releases/tags/${VERSION}"
TAG_URL="https://github.com/${SLUG}/releases/tag/${VERSION}"

# gh_get URL: sets GH_BODY from the response. Aborts the whole script on a rate
# limit (403/429) or an unreachable API, rather than masking it as "not ready".
GH_BODY=""
gh_get() {
	local out code auth=()
	[[ -n "${GITHUB_TOKEN:-}" ]] && auth=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
	out="$(curl -sSL "${auth[@]}" -w $'\n%{http_code}' "$1")" ||
		die "the GitHub API is unreachable"
	code="${out##*$'\n'}"
	GH_BODY="${out%$'\n'*}"
	if [[ "${code}" == "403" || "${code}" == "429" ]]; then
		warn "the release may have been made anyway, see ${TAG_URL}"
		die "GitHub API rate limit reached (HTTP ${code}): over the 60 requests/hr unauthenticated limit"
	fi
}

step "waiting for the build of ${SHA:0:8}"
conclusion=""
run_url=""
for ((i = 1; i <= RETRIES; i++)); do
	gh_get "${RUNS_URL}"
	conclusion="$(jq -r '.workflow_runs[0].conclusion // "pending"' <<<"${GH_BODY}" 2>/dev/null || echo pending)"
	run_url="$(jq -r '.workflow_runs[0].html_url // ""' <<<"${GH_BODY}" 2>/dev/null || echo "")"
	case "${conclusion}" in
	success) break ;;
	failure | cancelled | timed_out | startup_failure)
		die "the release build did not succeed (${conclusion})${run_url:+, see ${run_url}}"
		;;
	esac
	printf '    not finished yet (%d/%d) ...\n' "${i}" "${RETRIES}"
	sleep "${SLEEP}"
done
[[ "${conclusion}" == "success" ]] ||
	die "the build is still not finished after $((RETRIES * SLEEP))s (last status: ${conclusion:-unknown}), see ${TAG_URL}"

# The build succeeded, so one call is enough to confirm what it attached.
gh_get "${REL_URL}"
assets="$(jq '.assets | length' <<<"${GH_BODY}" 2>/dev/null || echo 0)"
[[ "${assets}" -ge "${EXPECTED}" ]] ||
	die "the build succeeded but ${VERSION} has only ${assets} of ${EXPECTED} assets, see ${TAG_URL}"

step "done: ${VERSION} is out with ${assets} assets"
echo "    ${TAG_URL}"
echo "    docker pull ${IMAGE}:${VERSION}"
