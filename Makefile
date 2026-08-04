# Makefile for the Go port of SqueakJS (sources under ./go).
#
# Common targets:
#   make build           compile the squeak binary into ./bin
#   make run             boot an image and open the live window (IMAGE=demo/mini.image)
#   make run-fullscreen  same, but fullscreen
#   make diag            just load an image and print diagnostics (no run)
#   make snap            boot headless and save the screen to desktop.png
#   make run-display     open the Ebitengine shell with the demo backend
#   make clean           remove build artifacts and the Go build cache
#   make test            run the Go tests
#   make fmt vet         format / vet the Go code

GO       ?= go
GO_DIR   := go
BIN_DIR  := bin
BINARY   := $(BIN_DIR)/squeak
PKG      := ./cmd/squeak
IMAGE    ?= demo/mini.image

# Absolute path so the binary can be run from the repo root.
BINARY_ABS := $(abspath $(BINARY))

.PHONY: all build run run-fullscreen diag run-display clean test fmt vet snap

all: build

build:
	mkdir -p $(BIN_DIR)
	cd $(GO_DIR) && $(GO) build -o $(BINARY_ABS) $(PKG)

# Boot the image and open the live, interactive window.
run: build
	$(BINARY_ABS) -run $(IMAGE)

run-fullscreen: build
	$(BINARY_ABS) -run -fullscreen $(IMAGE)

# Load an image and print diagnostics only (no execution).
diag: build
	$(BINARY_ABS) $(IMAGE)

run-display: build
	$(BINARY_ABS) -display

test:
	cd $(GO_DIR) && $(GO) test ./...

fmt:
	cd $(GO_DIR) && $(GO) fmt ./...

vet:
	cd $(GO_DIR) && $(GO) vet ./...

clean:
	cd $(GO_DIR) && $(GO) clean
	rm -rf $(BIN_DIR)

snap: build
	$(BINARY_ABS) -boot 0 -snap desktop.png $(IMAGE)
	@echo "wrote desktop.png"
