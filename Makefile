BINARY := jellyrpc
ARCH ?= amd64
DIST_DIR := dist
PACKAGES_DIR := $(DIST_DIR)/packages

.PHONY: all build build-linux build-darwin build-windows package-linux package-darwin package-windows release clean

all: release

build: build-linux build-darwin build-windows

build-linux:
	mkdir -p $(DIST_DIR)/linux
	GOOS=linux GOARCH=$(ARCH) go build -o $(DIST_DIR)/linux/$(BINARY) ./cmd/jellyrpc

build-darwin:
	mkdir -p $(DIST_DIR)/darwin
	GOOS=darwin GOARCH=$(ARCH) go build -o $(DIST_DIR)/darwin/$(BINARY) ./cmd/jellyrpc

build-windows:
	mkdir -p $(DIST_DIR)/windows
	GOOS=windows GOARCH=$(ARCH) go build -o $(DIST_DIR)/windows/$(BINARY).exe ./cmd/jellyrpc

package-linux: build-linux
	mkdir -p $(PACKAGES_DIR)
	cp jellyrpc.service $(DIST_DIR)/linux/jellyrpc.service
	cp jellyrpc.env.example $(DIST_DIR)/linux/jellyrpc.env.example
	tar -czf $(PACKAGES_DIR)/$(BINARY)-linux-$(ARCH).tar.gz -C $(DIST_DIR)/linux .

package-darwin: build-darwin
	mkdir -p $(PACKAGES_DIR)
	cp jellyrpc.service $(DIST_DIR)/darwin/jellyrpc.service
	cp jellyrpc.env.example $(DIST_DIR)/darwin/jellyrpc.env.example
	tar -czf $(PACKAGES_DIR)/$(BINARY)-darwin-$(ARCH).tar.gz -C $(DIST_DIR)/darwin .

package-windows: build-windows
	mkdir -p $(PACKAGES_DIR)
	cp jellyrpc.service $(DIST_DIR)/windows/jellyrpc.service
	cp jellyrpc.env.example $(DIST_DIR)/windows/jellyrpc.env.example
	(cd $(DIST_DIR)/windows && zip -r ../packages/$(BINARY)-windows-$(ARCH).zip .)

release: package-linux package-darwin package-windows

clean:
	rm -rf $(DIST_DIR)
