all: clean build

PKG = github.com/ChengyuZhu6/guest-pull-snapshotter
PACKAGES ?= $(shell go list ./...)
SUDO = $(shell which sudo)
GO_EXECUTABLE_PATH ?= $(shell which go)
GOOS ?= linux
GOARCH ?= $(shell go env GOARCH)
KERNEL_VER = $(shell uname -r)

BUILD_TIMESTAMP=$(shell date '+%Y-%m-%dT%H:%M:%S')
VERSION=$(shell git describe --match 'v[0-9]*' --dirty='.m' --always --tags)
REVISION=$(shell git rev-parse HEAD)$(shell if ! git diff --no-ext-diff --quiet --exit-code; then echo .m; fi)

RELEASE=guest-pull-snapshotter-v$(VERSION:v%=%)-${GOOS}-${GOARCH}

ifdef GOPROXY
PROXY := GOPROXY="${GOPROXY}"
endif

LDFLAGS = -s -w -X ${PKG}/version.Version=${VERSION} -X ${PKG}/version.Revision=$(REVISION) -X ${PKG}/version.BuildTimestamp=$(BUILD_TIMESTAMP)

.PHONY: build
build:
	GOOS=${GOOS} GOARCH=${GOARCH} ${PROXY} go build -ldflags "$(LDFLAGS)" -v -o bin/containerd-guest-pull-grpc ./cmd/containerd-guest-pull-grpc
	GOOS=${GOOS} GOARCH=${GOARCH} ${PROXY} go build -ldflags "$(LDFLAGS)" -v -o bin/guest-pull-overlayfs ./cmd/guest-pull-overlayfs
	chmod +x bin/*
.PHONY: clean
clean:
	rm -f bin/*
	rm -rf _out

.PHONY: install
install:
	@echo "+ $@ bin/containerd-guest-pull-grpc"
	@sudo install -D -m 755 bin/containerd-guest-pull-grpc /usr/local/bin/containerd-guest-pull-grpc
	@echo "+ $@ bin/guest-pull-overlayfs"
	@sudo install -D -m 755 bin/guest-pull-overlayfs /usr/local/bin/guest-pull-overlayfs

.PHONY: check
check:
	golangci-lint run

.PHONY: test
test:
	${PROXY} go test -v $(PACKAGES)

.PHONY: test-coverage
test-coverage:
	${PROXY} go test -v -coverprofile=coverage.out $(PACKAGES)
	go tool cover -html=coverage.out -o coverage.html
