# Build steps adapted from Notted's Makefile (itself from github.com/brienze1/quant, MIT); see NOTICE.md.
export PATH := $(HOME)/.gvm/pkgsets/go1.26.3/global/bin:$(HOME)/.nvm/versions/node/v24.16.0/bin:$(PATH)
.PHONY: dev build install release publish test test-go test-fe
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

# make release VERSION=v1.2.3 builds build/release (see scripts/release.sh). CI does this on each tag.
release:
	bash scripts/release.sh $(VERSION)

# make publish VERSION=v1.2.3 uploads build/release to a GitHub Release on that tag, as GabiHert.
publish:
	GH_TOKEN="$$(gh auth token -u GabiHert)" gh release create $(VERSION) build/release/* --repo GabiHert/deputy --title "Deputy $(VERSION)" --generate-notes --verify-tag

test-go:
	go vet ./...
	go test ./...

test-fe:
	cd frontend && npm ci && npm run typecheck && npm test

test: test-go test-fe
