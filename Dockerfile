#syntax=docker/dockerfile:1.9
ARG GO_IMAGE=golang:1.24-bullseye

FROM ${GO_IMAGE} AS build
ARG BIN=server
WORKDIR /src
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/${BIN} ./cmd/${BIN}

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
ARG BIN=server
COPY --from=build /out/${BIN} /app/${BIN}
USER nonroot:nonroot
EXPOSE 8080 9090
ENTRYPOINT ["/app/server"]
