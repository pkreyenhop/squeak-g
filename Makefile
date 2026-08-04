# Makefile for the Go port of SqueakJS (sources under ./go).
#
# Common targets:
#   make build        compile the squeak binary into ./bin
#   make run          load and run an image (IMAGE=demo/mini.image by default)
#   make run-display  open the Ebitengine display shell
#   make clean        remove build artifacts and the Go build cache for this module
#   make test         run the Go tests
#   make fmt vet      format / vet the Go code

GO       ?= go
GO_DIR   := go
BIN_DIR  := bin
BINARY   := $(BIN_DIR)/squeak
PKG      := ./cmd/squeak
IMAGE    ?= demo/mini.image

# Absolute path so the binary can be run from the repo root.
BINARY_ABS := $(abspath $(BINARY))

.PHONY: all build run run-display clean test fmt vet snap

all: build

build:
	mkdir -p $(BIN_DIR)
	cd $(GO_DIR) && $(GO) build -o $(BINARY_ABS) $(PKG)

run: build
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
