#!/bin/bash
set -e

version=$(git describe --tags 2>/dev/null || echo "dev")
builddate=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
IMAGE="${DOCKER_IMAGE:-ghcr.io/tlalocweb/tlaloc-backend}"
TAG="${DOCKER_TAG:-${version}}"

echo "Building tlaloc-backend version ${version} built on ${builddate}"

# Parse flags
ACTION=""
TAG_LATEST=false
for arg in "$@"; do
    case "${arg}" in
        --local)  ACTION="local" ;;
        --push)   ACTION="push" ;;
        --latest) TAG_LATEST=true ;;
        --help)   ACTION="help" ;;
        *)
            echo "Unknown option: ${arg}"
            ACTION="help"
            ;;
    esac
done

LATEST_TAG=""
if [ "${TAG_LATEST}" = true ]; then
    LATEST_TAG="--tag ${IMAGE}:latest"
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

case "${ACTION}" in
    local)
        echo "Building for local platform..."
        docker buildx build \
            --network=host \
            --load \
            -f "${SCRIPT_DIR}/Dockerfile" \
            --tag "${IMAGE}:${TAG}" \
            ${LATEST_TAG} \
            "${SCRIPT_DIR}"
        echo "Image built: ${IMAGE}:${TAG}"
        if [ "${TAG_LATEST}" = true ]; then
            echo "Also tagged: ${IMAGE}:latest"
        fi
        ;;
    push)
        echo "Building multi-platform and pushing..."
        docker buildx create --use --platform=linux/arm64,linux/amd64 --name multi-platform-builder 2>/dev/null || true
        docker buildx inspect --bootstrap
        docker buildx build \
            -f "${SCRIPT_DIR}/Dockerfile" \
            --platform linux/amd64,linux/arm64 \
            --tag "${IMAGE}:${TAG}" \
            ${LATEST_TAG} \
            --push \
            "${SCRIPT_DIR}"
        ;;
    *)
        echo "Usage: $0 <--local|--push> [--latest]"
        echo ""
        echo "  --local    Build for local platform only, loads into docker"
        echo "  --push     Build multi-platform (amd64+arm64) and push to registry"
        echo "  --latest   Also tag the image as :latest"
        echo ""
        echo "Examples:"
        echo "  $0 --local                Build with version tag only"
        echo "  $0 --local --latest       Build and also tag as :latest"
        echo "  $0 --push --latest        Build multi-platform, push with :latest"
        echo ""
        echo "Environment variables:"
        echo "  DOCKER_IMAGE   Image name (default: ghcr.io/tlalocweb/tlaloc-backend)"
        echo "  DOCKER_TAG     Image tag (default: git tag or 'dev')"
        ;;
esac
