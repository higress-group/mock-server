GOOS ?= $(shell go env GOOS)

# Git information
GIT_VERSION ?= $(shell git describe --tags --always)
GIT_COMMIT_HASH ?= $(shell git rev-parse HEAD)
GIT_TREESTATE = $(if $(shell git diff --quiet || echo 1), clean, dirty)
BUILDDATE = $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# Images management
REGISTRY ?= $(IMAGE_REGISTRY)
REGISTRY_NAMESPACE ?= higress

# Image URL to use all building/pushing image targets
VERSION ?= $(GIT_VERSION)
LL_MOCK_IMG ?= "${REGISTRY}/${REGISTRY_NAMESPACE}/llm-mock:${VERSION}"

## docker buildx support platform
PLATFORMS ?= linux/arm64,linux/amd64

.PHONY: image-buildx
image-buildx:  ## Build and push docker image for the llm-mock for cross-platform support
	docker buildx build --push --platform=$(PLATFORMS) --tag ${LL_MOCK_IMG} --build-arg BUILDPLATFORM=${BUILDPLATFORM} .
