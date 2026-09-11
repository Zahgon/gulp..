GO ?= go

.DEFAULT_GOAL := check

.PHONY: check
check: fmt-check vet lint test build

.PHONY: build
build:
	$(GO) build ./...
	$(GO) build -o /dev/null ./cmd/gulp

.PHONY: test
test:
	$(GO) test -race -covermode=atomic -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -n 1

# Skips the subprocess and filesystem-timing tests, which dominate the runtime.
.PHONY: test-short
test-short:
	$(GO) test -short ./...

.PHONY: cover
cover: test
	$(GO) tool cover -html=coverage.out

.PHONY: fmt
fmt:
	gofmt -w .

.PHONY: fmt-check
fmt-check:
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then echo "gofmt needed:"; echo "$$files"; exit 1; fi

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: lint
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed; skipping (see .golangci.yml)"; \
	fi

# Compares against a real gulp install. Point GULP_JS_REPO at a gulp checkout
# with node_modules present; the golden vectors in differential_test.go run
# either way.
.PHONY: differential
differential:
	$(GO) test -run Differential -v .

.PHONY: clean
clean:
	rm -f coverage.out
