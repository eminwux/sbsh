RELEASE_DIR := release

# ----- Version sourcing -----
MODULE := $(shell go list -m)

# CI can pass SBSH_VERSION (e.g., github.ref_name). If not, derive from git.
ifndef SBSH_VERSION
SBSH_VERSION = $(shell git describe --tags --always --dirty --match 'v*')
endif

# ----- Docker-related Variables (use the SAME version) -----
SBSH_REGISTRY ?= eminwux
SBSH_IMAGE_NAME := sbsh
SBSH_IMAGE_TAG ?= $(SBSH_VERSION)
SBSH_DOCKER_IMAGE := $(SBSH_REGISTRY)/$(SBSH_IMAGE_NAME):$(SBSH_IMAGE_TAG)

# ----- Build matrix -----
BINS = sbsh-sb
OS = linux darwin freebsd android
ARCHS = amd64 arm64


all: clean kill $(BINS)

.PHONY: release
release: release-build docker-build docker-push

sbsh-sb:
	go build \
	-o sbsh \
	-ldflags="-s -w -X $(MODULE)/cmd/config.Version=$(SBSH_VERSION)" \
	./cmd/
	ln sbsh sb

sbsh:
	go build -o sbsh ./cmd/sbsh

sb:
	go build -o sb ./cmd/sb


release-build:
	# Build for all OS and ARCH combinations
	for OS in $(OS); do \
		for ARCH in $(ARCHS); do \
			if [ "$$OS" = "android" ]; then \
				if [ "$$ARCH" != "arm64" ]; then \
					# Skipping: android builds are only compiled for arm64 architecture \
					continue; \
				fi; \
				BUILD_MODE="-buildmode=pie"; \
			else \
				BUILD_MODE=""; \
			fi; \
			GO111MODULE=on CGO_ENABLED=0 GOOS=$$OS GOARCH=$$ARCH \
			go build -a \
			-trimpath \
			$$BUILD_MODE \
			-o sbsh-$$OS-$$ARCH \
			-ldflags="-s -w -X $(MODULE)/cmd/config.Version=$(SBSH_VERSION)" \
			./cmd; \
		done \
	done

docker-build:
	# Only support Docker build for linux OS
	if [ "$(OS)" != "linux" ]; then \
		echo "Error: Docker images can only be built for linux OS. Current OS list: $(OS)"; \
	fi
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
	OS := linux
	# Build for all OS and ARCH combinations
	for OS in $(OS); do \
		for ARCH in $(ARCHS); do \
			echo "Pushing sbsh Docker image $(SBSH_REGISTRY)/$(SBSH_IMAGE_NAME):$(SBSH_VERSION)-$$OS-$$ARCH"; \
			docker push $(SBSH_REGISTRY)/$(SBSH_IMAGE_NAME):$(SBSH_VERSION)-$$OS-$$ARCH ; \
		done \
	done

clean:
	rm -rf $(HOME)/.sbsh/run/*
	rm -rf sbsh sb sb-sh

kill:
	(killall sbsh || true )

test:
	go test ./cmd/sb...
	go test ./cmd/sbsh...
	go test ./internal/terminal...
	go test ./internal/supervisor...
	go test -tags=integration ./cmd/sb/get/...
	E2E_BIN_DIR=$(shell pwd) go test ./e2e
	rm -rf e2e/tmp

e2e: test-e2e
.PHONY: test-e2e
test-e2e:
	@echo "Running e2e tests using binaries in project root"
	HOME=$(HOME) E2E_BIN_DIR=$(CURDIR) go test -v ./e2e -v

tag:
	git tag -s v$(SBSH_VERSION) -m "Release version $(SBSH_VERSION)"
	git push origin v$(SBSH_VERSION)
