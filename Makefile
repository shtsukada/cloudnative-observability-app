APP := cloudnative-observability-app
IMAGE ?= ghcr.io/stsukada/$(APP)
TAG ?= v0.1.0

GOOS ?= linux
GOARCH ?= amd64

.PHONY: build
build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o bin/server ./cmd/server
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o bin/client ./cmd/client

.PHONY: run-server
run-server:
  go run ./cmd/server

.PHONY: run-client
run-client:
  go run ./cmd/client

.PHONY: docker-build
docker-build:
	docker buildx build --platform linux/amd64 -t $(IMAGE):$(TAG) --build-arg BIN=server .

.PHONY: docker-push
docker-push:
	docker buildx build --platform linux/amd64 -t $(IMAGE):$(TAG) --build-arg BIN=server --push .

.PHONY: tidy
tidy:
  go mod tidy
