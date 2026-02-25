#!/usr/bin/env bash
set -euo pipefail

# Cross-platform release build for codex-switcher.
#
# Defaults:
# - Uses Docker (golang:1.22) for reproducible builds.
# - Outputs binaries to dist/releases.
#
# Optional env vars:
# - OUT_DIR=dist/releases
# - GO_DOCKER_IMAGE=golang:1.22
# - USE_DOCKER=1 (set to 0 for native local Go build)
# - FORCE_REBUILD=1 (uses `go build -a` to bypass build cache)
# - CLEAN=1 (removes previous binaries before building)

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${OUT_DIR:-dist/releases}"
GO_DOCKER_IMAGE="${GO_DOCKER_IMAGE:-golang:1.22}"
USE_DOCKER="${USE_DOCKER:-1}"
FORCE_REBUILD="${FORCE_REBUILD:-0}"
CLEAN="${CLEAN:-0}"

GO_BUILD_FLAGS="-trimpath -ldflags=-s -ldflags=-w"
if [[ "${FORCE_REBUILD}" == "1" ]]; then
  GO_BUILD_FLAGS="-a ${GO_BUILD_FLAGS}"
fi

if [[ "${CLEAN}" == "1" ]]; then
  rm -f "${ROOT_DIR}/${OUT_DIR}/codex-switcher-linux-x86_64" \
        "${ROOT_DIR}/${OUT_DIR}/codex-switcher-windows-x86_64.exe" \
        "${ROOT_DIR}/${OUT_DIR}/codex-switcher-macos-x86_64" \
        "${ROOT_DIR}/${OUT_DIR}/codex-switcher-macos-arm64"
fi

mkdir -p "${ROOT_DIR}/${OUT_DIR}"

build_native() {
  echo "Building with local Go toolchain..."
  (
    cd "${ROOT_DIR}"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ${GO_BUILD_FLAGS} -o "${OUT_DIR}/codex-switcher-linux-x86_64" ./cmd/codex-switcher
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ${GO_BUILD_FLAGS} -o "${OUT_DIR}/codex-switcher-windows-x86_64.exe" ./cmd/codex-switcher
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build ${GO_BUILD_FLAGS} -o "${OUT_DIR}/codex-switcher-macos-x86_64" ./cmd/codex-switcher
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ${GO_BUILD_FLAGS} -o "${OUT_DIR}/codex-switcher-macos-arm64" ./cmd/codex-switcher
  )
}

build_docker() {
  echo "Building with Docker image ${GO_DOCKER_IMAGE}..."
  (
    cd "${ROOT_DIR}"
    docker run --rm \
      -i \
      -u "$(id -u):$(id -g)" \
      -v "${ROOT_DIR}":/src \
      -w /src \
      "${GO_DOCKER_IMAGE}" \
      bash -s -- "${OUT_DIR}" "${GO_BUILD_FLAGS}" <<'EOF'
set -euo pipefail

OUT_DIR="$1"
GO_BUILD_FLAGS="$2"
export PATH="/usr/local/go/bin:$PATH"
export HOME=/tmp
export GOCACHE=/tmp/go-build
export GOPATH=/tmp/go
export GOMODCACHE=/tmp/go/pkg/mod

mkdir -p "${OUT_DIR}"

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ${GO_BUILD_FLAGS} -o "${OUT_DIR}/codex-switcher-linux-x86_64" ./cmd/codex-switcher
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ${GO_BUILD_FLAGS} -o "${OUT_DIR}/codex-switcher-windows-x86_64.exe" ./cmd/codex-switcher
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build ${GO_BUILD_FLAGS} -o "${OUT_DIR}/codex-switcher-macos-x86_64" ./cmd/codex-switcher
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ${GO_BUILD_FLAGS} -o "${OUT_DIR}/codex-switcher-macos-arm64" ./cmd/codex-switcher
EOF
  )
}

if [[ "${USE_DOCKER}" == "1" ]]; then
  if command -v docker >/dev/null 2>&1; then
    build_docker
  else
    echo "Docker not found, falling back to native build."
    build_native
  fi
else
  build_native
fi

echo
echo "Build artifacts in ${OUT_DIR}:"
ls -lh "${ROOT_DIR}/${OUT_DIR}"

echo
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "${ROOT_DIR}/${OUT_DIR}"/*
elif command -v shasum >/dev/null 2>&1; then
  shasum -a 256 "${ROOT_DIR}/${OUT_DIR}"/*
fi
