# dorgu-platform-v1
#
# Two build steps, in this order: the SPA is built into internal/webui/dist,
# then the Go binary embeds that directory. `make check` is what CI runs.

SHELL := /bin/bash
.DEFAULT_GOAL := help

BINARY      := dorgu-dashboard
CMD         := ./cmd/dorgu-dashboard
DIST        := internal/webui/dist
WEB         := web
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)
GOFILES     := $(shell git ls-files '*.go' 2>/dev/null)

# Package list, with web/ filtered out. npm packages sometimes ship Go source
# (flatted does), and `go list ./...` walks node_modules, so an unfiltered
# pattern makes the test output depend on which npm dependencies happen to be
# installed. Nothing under web/ is ever Go this repo owns.
PKGS        := $(shell go list ./... 2>/dev/null | grep -v '/web/')

##@ General

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
		/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Frontend

.PHONY: web-deps
web-deps: ## Install frontend dependencies
	cd $(WEB) && npm install

.PHONY: web
web: web-deps ## Build the SPA into internal/webui/dist
	cd $(WEB) && npm run build
	@$(MAKE) --no-print-directory dist-placeholder

.PHONY: dist-placeholder
dist-placeholder: ## Restore the go:embed placeholder in internal/webui/dist
	@# Vite empties its output directory, which removes the committed
	@# placeholder. Without a file in dist, `go:embed all:dist` matches nothing
	@# and `go build` fails on a tree that has only ever had the frontend built.
	@# Restoring it here keeps both states working.
	@printf '%s\n' \
		'The Vite build writes here. Committed so go:embed succeeds on a fresh clone.' \
		> $(DIST)/.gitkeep

.PHONY: web-lint
web-lint: web-deps ## Lint the frontend
	cd $(WEB) && npm run lint

.PHONY: web-typecheck
web-typecheck: web-deps ## Typecheck the frontend
	cd $(WEB) && npm run typecheck

.PHONY: web-dev
web-dev: web-deps ## Run the Vite dev server against a dashboard on :7171
	cd $(WEB) && npm run dev

##@ Backend

.PHONY: build-all
build-all: ## Compile every Go package
	go build $(PKGS)

.PHONY: build
build: ## Build the binary, embedding whatever is in internal/webui/dist
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) $(CMD)

.PHONY: all
all: web build ## Build the SPA and then the binary with it embedded

.PHONY: run
run: ## Run the dashboard against your current kubeconfig
	go run $(CMD)

.PHONY: fmt
fmt: ## Format Go sources
	gofmt -w $(GOFILES)

.PHONY: fmt-check
fmt-check: ## Fail if any Go source is unformatted
	@unformatted=$$(gofmt -l $(GOFILES)); \
	if [[ -n "$$unformatted" ]]; then \
		echo "not gofmt'd:"; echo "$$unformatted"; exit 1; \
	fi

.PHONY: vet
vet: ## Run go vet
	go vet $(PKGS)

.PHONY: test
test: ## Run the Go tests
	go test $(PKGS)

.PHONY: test-race
test-race: ## Run the Go tests with the race detector
	go test -race $(PKGS)

.PHONY: cover
cover: ## Report test coverage per package
	go test -cover $(PKGS)

##@ Verification

.PHONY: check
check: fmt-check vet test-race web-typecheck web-lint ## Everything CI runs

##@ Packaging

.PHONY: docker
docker: ## Build the container image
	docker build -t dorgu-dashboard:$(VERSION) .

.PHONY: clean
clean: ## Remove build output
	rm -rf bin $(DIST)/assets $(DIST)/index.html
	@$(MAKE) --no-print-directory dist-placeholder
