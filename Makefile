# Project-specific build entry points. Do not modify Caddy's upstream entry point.
GO ?= go
LDFLAGS := -s -w
SOURCE := ./cmd/caddys
DIST := dist-caddys

.PHONY: build test test-site dist-linux clean

# Canonical local compilation: ./caddys (./caddys.exe on Windows).
build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags='$(LDFLAGS)' -o caddys$(if $(filter windows,$(shell $(GO) env GOOS)),.exe,) $(SOURCE)

test:
	$(GO) test ./cmd/caddys/sitecli ./modules/caddyhttp ./modules/caddyhttp/fileserver
	cd tools/sanitize-assets && $(GO) test ./...

# End-to-end tests using a temporary Caddyfile and directories.
test-site: build
	./tools/test-caddys-site-cli.sh

# Official distributable binaries for our Linux installer, with reproducible names.
dist-linux:
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags='$(LDFLAGS)' -o $(DIST)/caddys-linux-amd64 $(SOURCE)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags='$(LDFLAGS)' -o $(DIST)/caddys-linux-arm64 $(SOURCE)
	gzip -n -9 -c $(DIST)/caddys-linux-amd64 > $(DIST)/caddys-linux-amd64.gz
	gzip -n -9 -c $(DIST)/caddys-linux-arm64 > $(DIST)/caddys-linux-arm64.gz
	cd $(DIST) && sha256sum caddys-linux-amd64.gz caddys-linux-arm64.gz > SHA256SUMS
	cd $(DIST) && sha256sum caddys-linux-amd64.gz > caddys-linux-amd64.gz.sha256
	cd $(DIST) && sha256sum caddys-linux-arm64.gz > caddys-linux-arm64.gz.sha256
	cp install/alpine.sh $(DIST)/install.sh
	chmod 0755 $(DIST)/install.sh

clean:
	rm -f caddys caddys.exe
	rm -rf $(DIST)
