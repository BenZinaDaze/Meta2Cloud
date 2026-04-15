#!/usr/bin/env sh
set -eu

IMAGE="${IMAGE:-benz1/meta2cloud}"
VERSION="${1:-}"

set -- docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t "${IMAGE}:latest"

if [ -n "${VERSION}" ]; then
  set -- "$@" -t "${IMAGE}:${VERSION}"
fi

set -- "$@" --push .

exec "$@"
