# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags='-s -w' \
    -o /out/lxd-agent-loader ./cmd/lxd-agent-loader

FROM scratch
COPY manifest.yaml /manifest.yaml
COPY lxd-agent.yaml /rootfs/usr/local/etc/containers/lxd-agent.yaml
COPY --from=build /out/lxd-agent-loader /rootfs/usr/local/lib/containers/lxd-agent/lxd-agent-loader
