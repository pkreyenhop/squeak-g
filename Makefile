# Makefile for the Go port of SqueakJS (sources under ./go).
#
# Common targets:
#   make run             boot an image and open the live window (IMAGE=demo/mini.image)
#   make run-fullscreen  same, but fullscreen
#   make eval EXPR='...'  print-it: evaluate a Smalltalk expression (default 3 + 4)
#   make snap            boot headless and save the screen to desktop.png
#   make diag            load an image and print diagnostics (no run)
#   make build           compile both binaries into ./bin
#   make clean           remove build artifacts and the Go build cache
#   make test            run the Go tests
#   make fmt vet         format / vet the Go code

GO         ?= go
GO_DIR     := go
BIN_DIR    := bin
BINARY     := $(BIN_DIR)/squeak
BINARY_GUI := $(BIN_DIR)/squeakgui
IMAGE      ?= demo/mini.image
EXPR       ?= 3 + 4

# Absolute paths so binaries run from the repo root.
BINARY_ABS     := $(abspath $(BINARY))
BINARY_GUI_ABS := $(abspath $(BINARY_GUI))

.PHONY: all build build-gui run run-fullscreen diag eval snap clean test fmt vet

all: build

# Headless VM (no GUI dependency).
build:
	mkdir -p $(BIN_DIR)
	cd $(GO_DIR) && $(GO) build -o $(BINARY_ABS) ./cmd/squeak

# Interactive GUI VM (links Ebitengine).
build-gui:
	mkdir -p $(BIN_DIR)
	cd $(GO_DIR) && $(GO) build -o $(BINARY_GUI_ABS) ./cmd/squeakgui

# Boot the image and open the live, interactive window.
run: build-gui
	$(BINARY_GUI_ABS) $(IMAGE)

run-fullscreen: build-gui
	$(BINARY_GUI_ABS) -fullscreen $(IMAGE)

# Print-it: evaluate a Smalltalk expression headlessly.
eval: build
	@$(BINARY_ABS) -eval '$(EXPR)' $(IMAGE)

# Boot headless and save the screen to desktop.png.
snap: build
	$(BINARY_ABS) -boot 0 -snap desktop.png $(IMAGE)
	@echo "wrote desktop.png"

# Load an image and print diagnostics only (no execution).
diag: build
	$(BINARY_ABS) $(IMAGE)

test:
	cd $(GO_DIR) && $(GO) test ./...

fmt:
	cd $(GO_DIR) && $(GO) fmt ./...

vet:
	cd $(GO_DIR) && $(GO) vet ./...

clean:
	cd $(GO_DIR) && $(GO) clean ./...
	rm -rf $(BIN_DIR)
