#!/usr/bin/env bash
# Builds dist/Volm-Setup-<version>.exe from build/windows-amd64/Volm.exe with Inno Setup 6.
#
#   bash scripts/build-installer.sh 1.2.3
set -euo pipefail

version=${1:?usage: build-installer.sh <version>}
root=$(cd "$(dirname "$0")/.." && pwd)

[[ -f $root/build/windows-amd64/Volm.exe ]] || { echo 'Run go tool mygo build -skip-installer first.' >&2; exit 1; }

iscc=""
for candidate in \
  "${ISCC:-}" \
  "C:/Program Files (x86)/Inno Setup 6/ISCC.exe" \
  "${LOCALAPPDATA:-}/Programs/Inno Setup 6/ISCC.exe"; do
  if [[ -n $candidate && -f $candidate ]]; then
    iscc=$candidate
    break
  fi
done
[[ -n $iscc ]] || { echo 'ISCC.exe of Inno Setup 6 not found; set ISCC to its path.' >&2; exit 1; }

script=$root/installer/volm.iss
if command -v cygpath >/dev/null; then
  script=$(cygpath -w "$script")
fi
# Git Bash would turn ISCC's /Q and /D options into paths.
MSYS2_ARG_CONV_EXCL='*' "$iscc" /Q "/DAppVersion=$version" "$script"
echo "dist/Volm-Setup-$version.exe"
