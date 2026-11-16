.PHONY: build build-linux release lint vuln check clean

VERSION ?= dev
LDFLAGS := -s -w -X github.com/Tech-S1/kiosk-display/internal/buildinfo.version=$(VERSION)
BIN := ./cmd/kiosk-display

build:
	go build -ldflags="$(LDFLAGS)" -o bin/kiosk-display $(BIN)

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/kiosk-display-linux-amd64 $(BIN)

release:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/kiosk-display-linux-amd64 $(BIN)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/kiosk-display-linux-arm64 $(BIN)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/kiosk-display-darwin-amd64 $(BIN)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/kiosk-display-darwin-arm64 $(BIN)
	cd dist && sha256sum kiosk-display-* > SHA256SUMS

lint:
	golangci-lint run ./...

vuln:
	govulncheck ./...

check: lint vuln

clean:
	rm -rf bin dist
