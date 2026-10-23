BINARY  := anyctr
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOBIN   ?= $(shell go env GOPATH)/bin

# Every platform anyctr targets. Apple's container only exists on darwin, but the
# binary itself is portable everywhere.
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

.PHONY: all build install test test-integration verify-caps vet fmt check docs dist clean cross

all: check build

build:
	go build -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/anyctr

install:
	go install -ldflags '$(LDFLAGS)' ./cmd/anyctr

test:
	go test ./...

# test-integration drives the built binary against a real runtime: it pulls an
# image, runs containers, builds, and cleans up after itself. Set ANYCTR_RUNTIME
# to target one runtime instead of the detected default.
test-integration:
	go test -tags integration -timeout 20m ./internal/cli/ -run 'TestContainer|TestLogs|TestBuild|TestStrict|TestCompose' -v

# verify-caps checks the capability matrix against the --help surface of every
# runtime installed here. It starts no containers, so it is safe to run anywhere
# the CLI exists — including machines where the engine is not running.
verify-caps:
	go test -tags conformance ./internal/cli/ -run TestCapabilityMatrix -v

vet:
	go vet ./...

fmt:
	gofmt -l -w .

check: vet test
	@test -z "$$(gofmt -l . )" || (echo "gofmt needed:"; gofmt -l .; exit 1)

# cross verifies that the CLI builds for every supported platform. It catches
# platform-specific mistakes (signal handling, path handling) without a CI matrix.
cross:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		echo "  $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch go build -o /dev/null ./... || exit 1; \
	done

# docs regenerates the capability matrix from the code, which is its only source
# of truth: the flags anyctr documents are the flags anyctr accepts.
docs: build
	./bin/$(BINARY) runtime capabilities --all --markdown > docs/CAPABILITIES.md
	@echo "wrote docs/CAPABILITIES.md"

# docs-check fails when the committed matrix has drifted from the code.
docs-check: build
	@./bin/$(BINARY) runtime capabilities --all --markdown > /tmp/anyctr-caps.md
	@diff -u docs/CAPABILITIES.md /tmp/anyctr-caps.md \
		|| (echo "docs/CAPABILITIES.md is stale — run 'make docs'"; exit 1)

dist:
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; \
		[ "$$os" = "windows" ] && ext=".exe"; \
		echo "  dist/$(BINARY)-$$os-$$arch$$ext"; \
		GOOS=$$os GOARCH=$$arch go build -ldflags '$(LDFLAGS)' \
			-o dist/$(BINARY)-$$os-$$arch$$ext ./cmd/anyctr || exit 1; \
	done

clean:
	rm -rf bin dist
