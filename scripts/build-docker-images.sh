#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/build-docker-images.sh [options]

Build the KPI backend and frontend Docker images without starting containers.

Options:
  --tag TAG             Image tag. Default: local
  --prefix PREFIX       Image name prefix. Default: kpi-system
  --platform PLATFORM   Optional build platform, e.g. linux/amd64
  --no-cache            Build without Docker layer cache
  --pull                Always pull newer base images
  --with-nginx          Also build the Nginx gateway image
  --skip-pull           Skip git pull and build the current checkout
  --push                Push both images after a successful build
  -h, --help            Show this help

Environment equivalents:
  IMAGE_TAG, IMAGE_PREFIX, BUILD_PLATFORM, DOCKER_NO_CACHE, DOCKER_PULL, DOCKER_PUSH, SKIP_GIT_PULL, BUILD_NGINX

Examples:
  scripts/build-docker-images.sh
  scripts/build-docker-images.sh --skip-pull
  scripts/build-docker-images.sh --with-nginx
  scripts/build-docker-images.sh --tag 2026.10.21
  scripts/build-docker-images.sh --prefix registry.example.com/team/kpi --tag v1
  scripts/build-docker-images.sh --prefix registry.example.com/team/kpi --tag v1 --push
  scripts/build-docker-images.sh --platform linux/amd64 --no-cache
EOF
}

image_tag="${IMAGE_TAG:-local}"
image_prefix="${IMAGE_PREFIX:-kpi-system}"
build_platform="${BUILD_PLATFORM:-}"
no_cache="${DOCKER_NO_CACHE:-false}"
pull_base="${DOCKER_PULL:-false}"
push_images="${DOCKER_PUSH:-false}"
skip_git_pull="${SKIP_GIT_PULL:-false}"
build_nginx="${BUILD_NGINX:-false}"

while (($# > 0)); do
  case "$1" in
    --tag)
      [[ $# -ge 2 ]] || { echo "Missing value for --tag" >&2; exit 2; }
      image_tag="$2"
      shift 2
      ;;
    --prefix)
      [[ $# -ge 2 ]] || { echo "Missing value for --prefix" >&2; exit 2; }
      image_prefix="${2%/}"
      shift 2
      ;;
    --platform)
      [[ $# -ge 2 ]] || { echo "Missing value for --platform" >&2; exit 2; }
      build_platform="$2"
      shift 2
      ;;
    --no-cache)
      no_cache=true
      shift
      ;;
    --pull)
      pull_base=true
      shift
      ;;
    --with-nginx)
      build_nginx=true
      shift
      ;;
    --skip-pull)
      skip_git_pull=true
      shift
      ;;
    --push)
      push_images=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if [[ "$skip_git_pull" != true ]]; then
  if ! git rev-parse --git-dir >/dev/null 2>&1; then
    echo "Not inside a Git repository; pass --skip-pull to build the current checkout." >&2
    exit 1
  fi
  echo "==> Pulling latest code with fast-forward only"
  git status --short
  git pull --ff-only
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is not installed or not available in PATH." >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "Docker daemon is not reachable. Start Docker/Colima and retry." >&2
  exit 1
fi

backend_image="${image_prefix}-backend:${image_tag}"
frontend_image="${image_prefix}-frontend:${image_tag}"
nginx_image="${image_prefix}-nginx:${image_tag}"

build_args=(build)
if [[ -n "$build_platform" ]]; then
  build_args+=(--platform "$build_platform")
fi
if [[ "$no_cache" == true ]]; then
  build_args+=(--no-cache)
fi
if [[ "$pull_base" == true ]]; then
  build_args+=(--pull)
fi

revision="$(git rev-parse --short HEAD 2>/dev/null || printf 'unknown')"
created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
common_labels=(
  --label "org.opencontainers.image.title=kpi-system"
  --label "org.opencontainers.image.revision=${revision}"
  --label "org.opencontainers.image.created=${created_at}"
)

build_image() {
  local dockerfile="$1"
  local image="$2"
  local description="$3"

  echo
  echo "==> Building ${description}: ${image}"
  docker "${build_args[@]}" \
    -f "$dockerfile" \
    -t "$image" \
    "${common_labels[@]}" \
    .
}

echo "Repository: $repo_root"
echo "Revision:   $revision"
echo "Tag:        $image_tag"
[[ -n "$build_platform" ]] && echo "Platform:   $build_platform"

build_image Dockerfile.backend "$backend_image" "backend"
build_image Dockerfile.frontend "$frontend_image" "frontend"
if [[ "$build_nginx" == true ]]; then
  build_image Dockerfile.nginx "$nginx_image" "nginx gateway"
fi

if [[ "$push_images" == true ]]; then
  echo
  echo "==> Pushing images"
  docker push "$backend_image"
  docker push "$frontend_image"
  if [[ "$build_nginx" == true ]]; then
    docker push "$nginx_image"
  fi
fi

echo
echo "Build complete:"
images=("$backend_image" "$frontend_image")
[[ "$build_nginx" == true ]] && images+=("$nginx_image")
for image in "${images[@]}"; do
  docker image ls "$image" --format 'table {{.Repository}}:{{.Tag}}\t{{.ID}}\t{{.CreatedSince}}\t{{.Size}}'
done
