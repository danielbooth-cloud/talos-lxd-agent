IMAGE ?= ghcr.io/danielbooth-cloud/talos-lxd-agent
VERSION ?= 0.1.0
ARCH ?= amd64
PLATFORM ?= linux/$(ARCH)
CONTAINER_TOOL ?= podman
TALOS_RELEASE ?= v1.13.7
# Official extensions to preserve from the current schematic
# (iscsi-tools, tailscale) plus any others you rely on.
OFFICIAL_EXTENSIONS ?= ghcr.io/siderolabs/iscsi-tools:v0.2.0 \
	ghcr.io/siderolabs/tailscale:1.98.8
OUT_DIR ?= _out
# Registry credentials for the imager container (go-containerregistry reads
# ~/.docker/config.json inside the container).
CONTAINER_AUTH_FILE ?= $(HOME)/.config/containers/auth.json

.PHONY: all test build push validate installer load clean

# Full local verification: unit tests, amd64 cross-compile, amd64 image, layout.
all: test build validate

test:
	go test ./...
	@mkdir -p $(OUT_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -trimpath -ldflags='-s -w' \
	  -o $(OUT_DIR)/lxd-agent-loader-$(ARCH) ./cmd/lxd-agent-loader
	@file $(OUT_DIR)/lxd-agent-loader-$(ARCH)

build:
	$(CONTAINER_TOOL) build --platform $(PLATFORM) --tag $(IMAGE):$(VERSION) .

push: build
	$(CONTAINER_TOOL) push $(IMAGE):$(VERSION)

validate: build
	@sh scripts/validate.sh $(IMAGE):$(VERSION) $(CONTAINER_TOOL) $(ARCH)

# Build a custom Talos installer that contains the official extensions plus
# this one. The resulting tarball can be loaded, pushed to a registry, and
# applied to running nodes with `talosctl upgrade --image <reference>`.
#
# The podman auth file is bind-mounted so the imager can pull private
# extension images (it reads /root/.docker/config.json).
installer: push
	@sh scripts/installer.sh \
		$(CURDIR)/$(OUT_DIR) \
		$(CONTAINER_TOOL) \
		$(CONTAINER_AUTH_FILE) \
		ghcr.io/siderolabs/imager:$(TALOS_RELEASE) \
		$(ARCH) \
		ghcr.io/siderolabs/installer:$(TALOS_RELEASE) \
		$(OFFICIAL_EXTENSIONS) \
		$(IMAGE):$(VERSION)
	@echo "Installer assets written to $(OUT_DIR)/"

load:
	$(CONTAINER_TOOL) load -i $(firstword $(wildcard $(OUT_DIR)/*installer*.tar))

clean:
	rm -rf $(OUT_DIR)
