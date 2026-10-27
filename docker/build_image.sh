#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
build="${YUE_BUILD_ROOT:-$HOME/dev/build/v-YuE2}"
registry="${REGISTRY:-swr.cn-southwest-2.myhuaweicloud.com/ictrek}"
sheet="";mode=full;reuse_app=false;gateway_only=false;gateway_base=""
while [[ $# -gt 0 ]]; do
 case "$1" in
  --sheet) sheet="$2";shift 2;;
  --build-only) mode=build;shift;;
  --reuse-app-artifacts) reuse_app=true;shift;;
  --gateway-only) gateway_only=true;shift;;
  --gateway-base-image) gateway_base="$2";shift 2;;
  --publish-only) mode=publish;shift;;
  *) echo 'Usage: build_image.sh --sheet <sheet> [--build-only|--publish-only] [--gateway-only]' >&2;exit 1;;
 esac
done
case "$sheet" in
 AMD_with_mxn100) arch=amd64;prefix=amd;cuda=OFF;base=ubuntu:22.04;;
 AMD_with_cuda) arch=amd64;prefix=amd_cuda;cuda=ON;base=ubuntu:22.04;;
 ARM_without_cuda) arch=arm64;prefix=arm;cuda=OFF;base=ubuntu:22.04;;
 ARM_with_cuda) arch=arm64;prefix=arm_cuda;cuda=ON;base=ubuntu:24.04;;
 l4t) arch=arm64;prefix=l4t;cuda=ON;base=ubuntu:22.04;;
 thor_spark) arch=arm64;prefix=thor_spark;cuda=ON;base=ubuntu:24.04;;
 *) echo 'Explicit supported --sheet required' >&2;exit 1;;
esac
actual="$(uname -m)";[[ "$arch" == amd64 && "$actual" == x86_64 || "$arch" == arm64 && "$actual" == aarch64 ]] || { echo 'Architecture mismatch' >&2;exit 1; }
artifacts="$build/artifacts-${prefix}"
image="$registry/v-yue2:${prefix}_$(date +%Y%m%d)"
if [[ "$mode" != publish ]]; then
 if [[ "$gateway_only" == true ]]; then
  # Keep the published native runtime byte-for-byte for a boundary-only fix.
  [[ "$gateway_base" == "$registry/v-yue2:${prefix}_"* ]] || { echo 'An explicit published base image for this exact profile is required' >&2;exit 1; }
  docker image inspect "$gateway_base" >/dev/null 2>&1 || docker pull "$gateway_base"
  [[ "$(docker image inspect --format '{{.Architecture}}' "$gateway_base")" == "$arch" ]] || { echo 'Gateway base architecture mismatch' >&2;exit 1; }
  mkdir -p "$artifacts/bin"
  if command -v go >/dev/null 2>&1; then
   (cd "$root/vos/gateway"; CGO_ENABLED=0 go build -trimpath -o "$artifacts/bin/v-yue2-gateway" .)
  else
   test -x "${YUE_GATEWAY_BIN:?Stage an architecture-matched gateway built on a Go build host}"
   cp "$YUE_GATEWAY_BIN" "$artifacts/bin/v-yue2-gateway"
  fi
 else
 mkdir -p "$build/source/.cargo" "$artifacts/web" "$artifacts/bin" "$artifacts/engine" "$artifacts/notices"
 rsync -a --exclude=.git --exclude=node_modules --exclude=target --exclude=/app/dist --exclude=/ictrek.app/dist "$root/" "$build/source/"
 cd "$build/source"
 if [[ "$reuse_app" == true ]]; then
  source_artifacts="${YUE_APP_ARTIFACTS:?--reuse-app-artifacts requires architecture-matched YUE_APP_ARTIFACTS}"
  test -x "$source_artifacts/bin/music-server"
  test -s "$source_artifacts/web/index.html"
  rsync -a "$source_artifacts/bin/" "$artifacts/bin/"
  rsync -a --delete "$source_artifacts/web/" "$artifacts/web/"
 else
 test -f "$build/sources/vendor/anyhow/Cargo.toml"
 # Source replacement configuration was prepared locally, not by Git on this host.
 python3 - "$build" <<'PY'
import sys
from pathlib import Path
b=Path(sys.argv[1]);s=(b/'vendor-config.toml').read_text();start=s.index('[source.vendored-sources]');s=s[:start]+'[source.vendored-sources]\ndirectory = "'+str(b/'sources/vendor')+'"\n';(b/'source/.cargo/config.toml').write_text(s)
PY
 export RUSTUP_TOOLCHAIN=stable CARGO_TARGET_DIR="$build/target"
 cargo build --release --offline --locked -p music-server
 cp "$build/target/release/music-server" "$artifacts/bin/"
 if command -v go >/dev/null 2>&1; then
  (cd vos/gateway; CGO_ENABLED=0 go build -trimpath -o "$artifacts/bin/v-yue2-gateway" .)
 else
  test -x "${YUE_GATEWAY_BIN:?Stage an architecture-matched gateway built on a Go build host}"
  cp "$YUE_GATEWAY_BIN" "$artifacts/bin/v-yue2-gateway"
 fi
 npm --prefix app ci --no-audit --no-fund
 # The upstream npm prebuild reads Git history; build hosts receive archives.
 (cd app; VITE_VOS=true npm exec -- vite build)
 rsync -a --delete app/dist/ "$artifacts/web/"
 fi
 cp THIRD_PARTY_NOTICES.md "$artifacts/web/"
 cp -a vos/licenses "$artifacts/web/"
 cp -a licenses/. "$artifacts/web/licenses/"
 cp LICENSE "$artifacts/web/licenses/YuE2-Studio-MIT.txt"
 if [[ "$reuse_app" == false ]]; then python3 vos/scripts/collect_notices.py --source "$build/source" --vendor "$build/sources/vendor" --output "$artifacts/web/licenses/dependencies"; fi
 python3 vos/scripts/patch_engine.py "$build/sources/yue2.cpp"
 engine_build="${YUE_ENGINE_BUILD_DIR:-$build/engine-${prefix}}"
 cmake -S "$build/sources/yue2.cpp" -B "$engine_build" -DCMAKE_BUILD_TYPE=Release \
  -DGGML_NATIVE=OFF -DGGML_CUDA="$cuda" -DGGML_VULKAN=OFF -DGGML_BACKEND_DL=OFF \
  -DCMAKE_BUILD_RPATH='$ORIGIN' -DCMAKE_INSTALL_RPATH='$ORIGIN' -DCMAKE_BUILD_WITH_INSTALL_RPATH=ON
 cmake --build "$engine_build" --parallel "${YUE_BUILD_JOBS:-8}"
 find "$engine_build" -type f -name yue-server -exec cp {} "$artifacts/engine/" \;
 find "$engine_build" -name 'lib*.so*' -exec cp -a {} "$artifacts/engine/" \;
 if [[ "$cuda" == ON ]]; then
  # Ship the CUDA runtime used by this compiler, never the development SDK or
  # a Python/PyTorch image. Drivers remain supplied by the host's NVIDIA runtime.
  cuda_root="${CUDA_HOME:-/usr/local/cuda}"
  for name in libcudart libcublas libcublasLt libnvJitLink libnvrtc; do
   library="$cuda_root/lib64/$name.so"
   [[ -f "$library" ]] || continue
   soname="$(readelf -d "$library" | sed -n 's/.*SONAME.*\[\([^]]*\)\].*/\1/p')"
   [[ -n "$soname" ]] || { echo "No SONAME: $library" >&2;exit 1; }
   cp -L "$library" "$artifacts/engine/$soname"
  done
 fi
 cp -a LICENSE THIRD_PARTY_NOTICES.md licenses vos/licenses "$artifacts/notices/"
 cp "$build/sources/yue2.cpp/LICENSE" "$artifacts/notices/yue2.cpp-LICENSE"
 cp "$build/sources/yue2.cpp/ggml/LICENSE" "$artifacts/notices/ggml-LICENSE"
 fi
 # Explicitly reject weights in the runtime context; files may only arrive at runtime.
 if find "$artifacts" -type f \( -name '*.gguf' -o -name '*.safetensors' -o -name '*.onnx' \) | grep -q .; then echo 'Weights found in image artifacts' >&2;exit 1;fi
 if [[ "$gateway_only" == true ]]; then
 docker buildx build --builder default --load --provenance=false --sbom=false \
  --build-arg "RUNTIME_IMAGE=$gateway_base" --build-arg "APP_VERSION=$(cat ictrek.app/VERSION)" \
  -f "$root/docker/Dockerfile.gateway" -t "$image" "$artifacts/bin"
 else
 docker buildx build --builder default --load --provenance=false --sbom=false \
  --build-arg "RUNTIME_BASE=$base" --build-arg "APP_VERSION=$(cat ictrek.app/VERSION)" \
  -f "$root/docker/Dockerfile" -t "$image" "$artifacts"
 fi
fi
if [[ "$mode" != build ]]; then
 docker push "$image"
 python3 "$root/vos/scripts/feishu_release.py" --component v-yue2 --sheet "$sheet" --tag "${image##*:}" --repository "$registry/v-yue2"
fi
echo "$image"
