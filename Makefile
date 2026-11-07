# Build steps adapted from Notted's Makefile (itself from github.com/brienze1/quant, MIT); see NOTICE.md.
export PATH := $(HOME)/.gvm/pkgsets/go1.26.3/global/bin:$(HOME)/.nvm/versions/node/v24.16.0/bin:$(PATH)
.PHONY: dev build install release test test-go test-fe
APP := build/bin/Deputy.app
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

dev:
	wails dev

build:
	MACOSX_DEPLOYMENT_TARGET=14.0 CGO_CFLAGS=-mmacosx-version-min=14.0 CGO_LDFLAGS=-mmacosx-version-min=14.0 wails build -platform darwin/arm64 -ldflags "-X github.com/GabiHert/deputy/internal/run.Version=$(VERSION)"
	touch frontend/dist/.gitkeep
	bash scripts/fixup-macos-dylibs.sh $(APP)/Contents/MacOS/Deputy
	bash scripts/sign-bundle.sh $(APP)
	bash scripts/check-bundle.sh

install:
	bash scripts/install.sh

# make release builds Deputy, tags it and publishes a GitHub Release (see scripts/release.sh).
# make release V=v0.3.0 picks the version; without V it is the next patch after the newest tag.
release:
	bash scripts/release.sh $(V)

test-go:
	go vet ./...
	go test ./...

test-fe:
	cd frontend && npm ci && npm run typecheck && npm test

test: test-go test-fe
