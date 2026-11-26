#!/usr/bin/env bash
# Checks the release decisions of next-version.sh and select-ref.sh against a throwaway repository.
set -euo pipefail

scripts=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
cd "$work"
git init -q
git config user.name 'Release Rules Test'
git config user.email 'release-rules@example.invalid'
git commit -q --allow-empty -m 'Start'

expect_version() {
  local version=$1 preid=$2 tag=$3 retry=${4:-false} result
  result=$(VERSION="$version" PREID="$preid" bash "$scripts/next-version.sh")
  if [[ $result != "tag=$tag"$'\n'"retry=$retry" ]]; then
    printf 'VERSION=%s PREID=%s: expected %s (retry=%s), got %s\n' "$version" "$preid" "$tag" "$retry" "$result" >&2
    exit 1
  fi
}

expect_error() {
  local version=$1 preid=$2 message=$3 result
  if result=$(VERSION="$version" PREID="$preid" bash "$scripts/next-version.sh" 2>&1) || [[ $result != *"$message"* ]]; then
    printf 'VERSION=%s PREID=%s: expected an error containing %s, got %s\n' "$version" "$preid" "$message" "$result" >&2
    exit 1
  fi
}

git tag v-not-a-version
expect_version minor '' v0.1.0
git tag v0.0.0
expect_version 0.0.0 '' v0.0.0 true
git tag v1.0.0
expect_version minor rc v1.1.0-rc.0
expect_version prerelease rc v1.0.1-rc.0
expect_error prerelease '' 'a prerelease needs an identifier'
expect_error minor RC 'preid must contain lowercase letters only'
expect_error 1.1.0 beta 'leave preid empty'
git tag v1.1.0-beta.2
expect_version prerelease rc v1.1.0-rc.0
git tag v1.1.0-rc.0
expect_version prerelease '' v1.1.0-rc.1
expect_error prerelease beta 'must be newer'
git tag v1.1.0-rc.1
expect_version minor '' v1.1.0
git tag v1.1.0
expect_version 1.2.0-rc '' v1.2.0-rc.0
git tag v1.2.0-rc.1
expect_version 1.2.0-rc '' v1.2.0-rc.2
expect_version v1.2.0-rc.1 '' v1.2.0-rc.1 true
expect_error 1.0.5 '' 'must be newer'
expect_version major '' v2.0.0
tagged_commit=$(git rev-parse 'v1.1.0^{commit}')
[[ $(TAG=v1.1.0 RETRY=true bash "$scripts/select-ref.sh") == "ref=$tagged_commit" ]]
[[ $(TAG=v1.3.0 RETRY=false bash "$scripts/select-ref.sh") == "ref=$(git rev-parse HEAD)" ]]
echo 'Release decisions passed.'
