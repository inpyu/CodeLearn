GO ?= go
BINARY ?= codelearn
VERSION := $(shell sed -n 's/^const Version = "\(.*\)"/\1/p' internal/cli/cli.go)
PLATFORMS := linux/amd64 linux/arm64 darwin/arm64 windows/amd64

.PHONY: all build build-go frontend test test-go test-frontend test-e2e vet fmt clean release run

# The default target produces a single binary with the viewer embedded.
all: build

# The Vite build restores web/dist/.gitkeep itself, so this needs no wrapper.
frontend:
	cd frontend && npm ci && npm run build

build: frontend
	$(GO) build -o $(BINARY) ./cmd/codelearn

# Builds the binary without rebuilding the frontend, for backend-only work.
build-go:
	$(GO) build -o $(BINARY) ./cmd/codelearn

run: build-go
	./$(BINARY) .

vet:
	$(GO) vet ./...

fmt:
	gofmt -l -w cmd internal web

test: test-go test-frontend

test-go:
	$(GO) test ./...

test-frontend:
	cd frontend && npm run test -- --run

# The acceptance scenario, end to end, with no LLM involved.
test-e2e: build
	./tests/e2e.sh ./$(BINARY)

# Cross compiles a release binary per target platform.
release: frontend
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		out=dist/$(BINARY)-$(VERSION)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then out=$$out.exe; fi; \
		echo "building $$out"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(GO) build -trimpath -o $$out ./cmd/codelearn || exit 1; \
	done

clean:
	rm -rf $(BINARY) dist web/dist/assets web/dist/index.html frontend/node_modules
