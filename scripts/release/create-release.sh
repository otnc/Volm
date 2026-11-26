#!/usr/bin/env bash
# Creates the GitHub release of $TAG with the files in dist/, or, when retrying, uploads them again to the existing release.
set -euo pipefail

if gh release view "$TAG" >/dev/null 2>&1; then
  gh release upload "$TAG" dist/* --clobber
  exit 0
fi

flags=(--verify-tag --generate-notes)
if [[ $TAG == *-* ]]; then
  flags+=(--prerelease)
fi
gh release create "$TAG" "${flags[@]}" dist/*
