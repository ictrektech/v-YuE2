#!/usr/bin/env bash
# Run locally; remote build machines never contact Git repositories.
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
out="${YUE_BUILD_ROOT:-$HOME/dev/build/v-YuE2}"
mkdir -p "$out/sources"
cd "$root"
RUSTUP_TOOLCHAIN=stable cargo vendor --locked "$out/sources/vendor" > "$out/vendor-config.toml"
if [[ ! -d "$out/sources/yue2.cpp/.git" ]]; then
  git clone --recurse-submodules https://github.com/timoncool/yue2.cpp.git "$out/sources/yue2.cpp"
fi
commit="$(python3 -c 'import json; print(json.load(open("engines/yue2-cpp-source.json"))["commit"])')"
git -C "$out/sources/yue2.cpp" checkout "$commit"
git -C "$out/sources/yue2.cpp" submodule update --init --recursive
echo "Prepared source dependencies at $out/sources; rsync them to the build host."
