RELEASE_DIR := release

# Define federid version
SBSH_VERSION = 0.1.0

# ------------------------------
# Docker-related Variables
# ------------------------------

# Webhook Docker image settings
SBSH_REGISTRY ?= eminwux
SBSH_IMAGE_NAME := sbsh
SBSH_IMAGE_TAG ?= $(SBSH_VERSION)
SBSH_DOCKER_IMAGE := $(SBSH_REGISTRY)/$(SBSH_IMAGE_NAME):$(SBSH_IMAGE_TAG)

BINS = sbsh sb
OS = linux darwin freebsd
ARCHS = amd64 arm64

all: clean kill $(BINS)


.PHONY: release
release: release-build docker-build docker-push


sbsh:
	go build -o sbsh ./cmd/sbsh

sb:
	go build -o sb ./cmd/sb


release-build:
	# Build for all OS and ARCH combinations
	for OS in $(OS); do \
		for ARCH in $(ARCHS); do \
			GO111MODULE=on CGO_ENABLED=0 GOOS=$$OS GOARCH=$$ARCH go build -a -o sb-$$OS-$$ARCH ./cmd/sb; \
			GO111MODULE=on CGO_ENABLED=0 GOOS=$$OS GOARCH=$$ARCH go build -a -o sbsh-$$OS-$$ARCH ./cmd/sbsh; \
		done \
	done

docker-build:
	# Only support Docker build for linux OS
	if [ "$(OS)" != "linux" ]; then \
		echo "Error: Docker images can only be built for linux OS. Current OS list: $(OS)"; \
	fi
	OS = linux
	# Build for all OS and ARCH combinations
	for OS in $(OS); do \
		for ARCH in $(ARCHS); do \
			echo "Building sbsh Docker image $(SBSH_REGISTRY)/$(SBSH_IMAGE_NAME):$(SBSH_VERSION)-$$OS-$$ARCH"; \
			docker build --build-arg ARCH=$$ARCH --build-arg OS=$$OS -t $(SBSH_REGISTRY)/$(SBSH_IMAGE_NAME):$(SBSH_VERSION)-$$OS-$$ARCH -f Dockerfile .; \
		done \
	done

docker-push:
	# Only support Docker build for linux OS
	if [ "$(OS)" != "linux" ]; then \
		echo "Error: Docker images can only be pushed for linux OS. Current OS variable: $(OS)"; \
	fi
	OS = linux
	# Build for all OS and ARCH combinations
	for OS in $(OS); do \
		for ARCH in $(ARCHS); do \
			echo "Pushing sbsh Docker image $(SBSH_REGISTRY)/$(SBSH_IMAGE_NAME):$(SBSH_VERSION)-$$OS-$$ARCH"; \
			docker push $(SBSH_REGISTRY)/$(SBSH_IMAGE_NAME):$(SBSH_VERSION)-$$OS-$$ARCH ; \
		done \
	done

clean:
	rm -rf sbsh sb

kill:
	(killall sbsh || true )

test:
	go test ./cmd/sb...
	go test ./cmd/sbsh...
	go test ./internal/session...
	go test ./internal/supervisor...
