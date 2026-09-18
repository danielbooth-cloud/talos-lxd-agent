IMAGE ?= ghcr.io/danielbooth-cloud/talos-lxd-agent
VERSION ?= 0.1.0
PLATFORM ?= linux/amd64
CONTAINER_TOOL ?= podman
TALOS_RELEASE ?= v1.13.7
# Official extensions to preserve from the current schematic
# (iscsi-tools, tailscale) plus any others you rely on.
OFFICIAL_EXTENSIONS ?= ghcr.io/siderolabs/iscsi-tools:v0.2.0 \
	ghcr.io/siderolabs/tailscale:v1.98.8
OUT_DIR ?= _out

.PHONY: test build push validate installer load clean

test:
	go test ./...
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /dev/null ./cmd/lxd-agent-loader

build:
	$(CONTAINER_TOOL) build --platform $(PLATFORM) --tag $(IMAGE):$(VERSION) .

push: build
	$(CONTAINER_TOOL) push $(IMAGE):$(VERSION)

validate: build
	@sh scripts/validate.sh $(IMAGE):$(VERSION) $(CONTAINER_TOOL)

# Build a custom Talos installer that contains the official extensions plus
# this one. The resulting tarball can be loaded, pushed to a registry, and
# applied to running nodes with `talosctl upgrade --image <reference>`.
installer: push
	mkdir -p $(OUT_DIR)
	$(CONTAINER_TOOL) run --rm -v $(CURDIR)/$(OUT_DIR):/out \
	  ghcr.io/siderolabs/imager:$(TALOS_RELEASE) \
	  installer \
	  --arch amd64 \
	  --platform metal \
	  --base-installer-image ghcr.io/siderolabs/installer:$(TALOS_RELEASE) \
	  $(foreach ext,$(OFFICIAL_EXTENSIONS),--system-extension-image $(ext) ) \
	  --system-extension-image $(IMAGE):$(VERSION) \
	  --output /out
	@echo "Installer assets written to $(OUT_DIR)/"

load:
	$(CONTAINER_TOOL) load -i $(firstword $(wildcard $(OUT_DIR)/*installer*.tar))

clean:
	rm -rf $(OUT_DIR)
