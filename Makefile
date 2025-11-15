APP := cloudnative-observability-app
IMAGE ?= ghcr.io/stsukada/$(APP)
TAG ?= v0.1.0

GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0

BIN_DIR := bin
SERVER_BIN := $(BIN_DIR)/server
CLIENT_BIN := $(BIN_DIR)/client

.PHONY: all
all: build

## build--------------------------------

.PHONY: build
build:
	mkdir -p $(BIN_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o $(SERVER_BIN) ./cmd/server
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o $(CLIENT_BIN) ./cmd/client

.PHONY: run-server
run-server:
	go run ./cmd/server

.PHONY: run-client
run-client:
	go run ./cmd/client

## Quality--------------------------------

.PHONY: fmt
fmt:
	gofmt -w ./cmd

.PHONY: test
test:
	go test ./..

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: tidy
tidy:
	go mod tidy

## Container build / push-----------------

.PHONY: docker-build
docker-build:
	@if [ "$(TAG)" = "latest" ]; then \
		echo "ERROR: TAG=latest is forbidden. specify TAG=SemVer"; \
		exit 2; \
	fi
	docker buildx build \
		--platform linux/amd64 \
		-t $(IMAGE):$(TAG) \
		--build-arg BIN=server \
		.

.PHONY: docker-push
docker-push:
	@if [ "$(TAG)" = "latest" ]; then \
		echo "ERROR: TAG=latest is forbidden. specify TAG=SemVer"; \
		exit 2; \
	fi
	docker buildx build \
		--platform linux/amd64 \
		-t $(IMAGE):$(TAG) \
		--build-arg BIN=server \
		--push \
		.
