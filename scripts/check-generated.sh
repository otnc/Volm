#!/usr/bin/env bash
# Fails when the icons, the logo or THIRD_PARTY_NOTICES.md differ from what their generators write now.
set -euo pipefail

generated=(assets internal/settings/appicon.png internal/settings/logo.png THIRD_PARTY_NOTICES.md)
go run ./tools/genicons
go run ./tools/gennotices
if ! git diff --exit-code --stat -- "${generated[@]}" || [[ -n $(git status --porcelain --untracked-files=all -- "${generated[@]}") ]]; then
  printf 'Generated files are out of date. Run go run ./tools/genicons and go run ./tools/gennotices, then commit the result.\n' >&2
  exit 1
fi
