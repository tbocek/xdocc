#!/usr/bin/env bash
#
# Cut a release. Versions count up one at a time - v1, v2, v3 - so a release
# needs no decision about what to call it, and there is nothing to pass: this
# tags the next number.
#
# The build happens on GitHub Actions, from the tag. This tags, pushes, and then
# waits for the workflow to finish, so you learn here whether the release is
# good rather than by looking later.
#
# Each version is tagged twice: vN is the version, and v1.N.0 is the same commit
# under a name Go's module system accepts. The major stays 1 and only the minor
# counts up, because the module has no importable API to break - everything is
# under internal/, and cmd/xdocc is a main package - so there is never a v2 to
# declare, and the module path never has to carry one.
#
# Needs: git, curl, jq.

set -euo pipefail

readonly SLUG="tbocek/xdocc"
readonly WORKFLOW="build.yml"
readonly IMAGE="ghcr.io/tbocek/xdocc"
readonly EXPECTED=8 # seven archives and the checksums file
readonly RETRIES=40
readonly SLEEP=15

die() { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }
step() { printf '\033[1m==>\033[0m %s\n' "$*"; }

cd "$(dirname "${BASH_SOURCE[0]}")"

[[ $# -eq 0 ]] || die "the next version comes from the tags, there is nothing to pass"
for cmd in git curl jq; do
	command -v "${cmd}" >/dev/null || die "${cmd} is not installed"
done
[[ -z "$(git status --porcelain)" ]] ||
	die "the working tree is not clean, commit or stash first"

# ---------------------------------------------------------------- version

# the next version is one past the highest that exists anywhere
git fetch --tags --quiet origin

# Highest by number, not by history: "git describe" answers with whatever tag
# is nearest, which is the wrong one if a tag was ever made out of order.
# "|| true" so that no tags at all is an empty answer and not a failure.
PREVIOUS="$(git tag -l 'v[0-9]*' | grep -x 'v[0-9]\+' | sort -V | tail -1 || true)"
PREVIOUS="${PREVIOUS:-v0}"
# 10# so that a v08 is eight and not a bad octal number
readonly VERSION="v$((10#${PREVIOUS#v} + 1))"
# the release number is the minor version: v3 is v1.3.0, v10 is v1.10.0
readonly SEMVER="v1.${VERSION#v}.0"

for tag in "${VERSION}" "${SEMVER}"; do
	if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
		die "tag ${tag} already exists"
	fi
done

# ---------------------------------------------------------------- tag

step "tagging ${VERSION}"
git tag -a "${VERSION}" -m "xdocc ${VERSION}"
git push --quiet origin "${VERSION}"

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
# A GITHUB_TOKEN in the environment raises the limit from 60/hr to 5000/hr,
# which matters because one release costs up to 41 calls of the 60.
GH_BODY=""
gh_get() {
	local out code auth=()
	[[ -n "${GITHUB_TOKEN:-}" ]] && auth=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
	out="$(curl -sSL "${auth[@]}" -w $'\n%{http_code}' "$1")" ||
		die "the GitHub API is unreachable"
	code="${out##*$'\n'}"
	GH_BODY="${out%$'\n'*}"
	if [[ "${code}" == "403" || "${code}" == "429" ]]; then
		printf '\033[33mwarning:\033[0m the release may have been made anyway, see %s\n' "${TAG_URL}" >&2
		die "GitHub API rate limit reached (HTTP ${code}): over the 60 requests/hr unauthenticated limit"
	fi
}

# Stage 1: wait for the run to finish. ${SEMVER} lands on this same commit, but
# it is pushed further down, so this commit has exactly one run to look at.
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

# Stage 2: the build succeeded, so one call confirms what it attached.
gh_get "${REL_URL}"
assets="$(jq '.assets | length' <<<"${GH_BODY}" 2>/dev/null || echo 0)"
[[ "${assets}" -ge "${EXPECTED}" ]] ||
	die "the build succeeded but ${VERSION} has only ${assets} of ${EXPECTED} assets, see ${TAG_URL}"

# Only now: a build that failed leaves no version for Go to find, and the
# workflow triggers on v*, so pushing this while the run was still being waited
# on would have put a second run on the same commit.
step "tagging ${SEMVER}, the name go install resolves"
git tag -a "${SEMVER}" -m "xdocc ${VERSION}"
git push --quiet origin "${SEMVER}"

step "done: ${VERSION} is out with ${assets} assets"
echo "    ${TAG_URL}"
echo "    docker pull ${IMAGE}:${VERSION}"
echo "    go install github.com/${SLUG}/cmd/xdocc@${SEMVER}"
