GO           ?= go
GOFMT        ?= $(GO)fmt
GOOPTS       ?=
GO111MODULE  :=
GOLANGCI_LINT_VERSION ?= v2.12.2
pkgs          = ./...

all: style vet build test

.PHONY: print-golangci-lint-version
print-golangci-lint-version:
	@echo "$(GOLANGCI_LINT_VERSION)"

.PHONY: build
build:
	@echo ">> building ping"
	GO111MODULE=$(GO111MODULE) $(GO) build $(GOOPTS) ./cmd/ping

.PHONY: style
style:
	@echo ">> checking code style"
	@fmtRes=$$($(GOFMT) -d $$(find . -path ./vendor -prune -o -name '*.go' -print)); \
	if [ -n "$${fmtRes}" ]; then \
		echo "gofmt checking failed!"; echo "$${fmtRes}"; echo; \
		echo "Please ensure you are using $$($(GO) version) for formatting code."; \
		exit 1; \
	fi

.PHONY: test
test:
	@echo ">> running all tests"
	GO111MODULE=$(GO111MODULE) $(GO) test -race -cover $(GOOPTS) $(pkgs)

.PHONY: vet
vet:
	@echo ">> vetting code"
	GO111MODULE=$(GO111MODULE) $(GO) vet $(GOOPTS) $(pkgs)
