# EarsAudioBridge Makefile
# `make` builds every client and server for all supported platforms into builds/.
# All builds are CGO_ENABLED=0; -j works (targets are real files).

GO         ?= go
BINDIR     ?= builds
BUILDFLAGS ?= -trimpath
LDFLAGS    ?= -s -w
LDFLAGS_WINDOWS ?= -s -w -H windowsgui

HOSTOS   := $(shell $(GO) env GOOS)
HOSTARCH := $(shell $(GO) env GOARCH)

# Client runs everywhere; server has no macOS runtime support so it is not built there.
CLIENT_PLATFORMS ?= linux/amd64 linux/arm64 windows/amd64 darwin/amd64 darwin/arm64
SERVER_PLATFORMS ?= linux/amd64 linux/arm64 windows/amd64

# Primary release names (no OS/arch suffix):
#   linux/amd64  -> builds/earsaudiobridge-{server,client}
#   windows/amd64 -> builds/earsaudiobridge-{server,client}.exe
# Other platforms -> builds/<cmd>_<os>_<arch>[.exe]
exe            = $(if $(filter windows,$(1)),.exe,)
binpath_suffix = $(BINDIR)/$(1)_$(subst /,_,$(2))$(call exe,$(firstword $(subst /, ,$(2))))
binpath        = $(if $(filter linux/amd64,$(2)),$(BINDIR)/$(1),$(if $(filter windows/amd64,$(2)),$(BINDIR)/$(1).exe,$(call binpath_suffix,$(1),$(2))))

CLIENT_BINS := $(foreach p,$(CLIENT_PLATFORMS),$(call binpath,earsaudiobridge-client,$(p)))
SERVER_BINS := $(foreach p,$(SERVER_PLATFORMS),$(call binpath,earsaudiobridge-server,$(p)))

.PHONY: all clients servers host linux windows darwin vet tidy checksums run-server run-client clean help FORCE

all: clients servers ## Build everything (default)

clients: $(CLIENT_BINS) ## Build all clients

servers: $(SERVER_BINS) ## Build all servers

linux: $(BINDIR)/earsaudiobridge-server $(BINDIR)/earsaudiobridge-client \
       $(BINDIR)/earsaudiobridge-server_linux_arm64 $(BINDIR)/earsaudiobridge-client_linux_arm64 ## Build all Linux binaries

windows: $(BINDIR)/earsaudiobridge-server.exe $(BINDIR)/earsaudiobridge-client.exe ## Build all Windows binaries

darwin: $(BINDIR)/earsaudiobridge-client_darwin_amd64 $(BINDIR)/earsaudiobridge-client_darwin_arm64 ## Build all macOS binaries

ifeq ($(HOSTOS)/$(HOSTARCH),linux/amd64)
host: $(BINDIR)/earsaudiobridge-client $(BINDIR)/earsaudiobridge-server ## Build client+server for this machine only
else ifeq ($(HOSTOS)/$(HOSTARCH),windows/amd64)
host: $(BINDIR)/earsaudiobridge-client.exe $(BINDIR)/earsaudiobridge-server.exe
else ifeq ($(HOSTOS),darwin)
host: $(call binpath,earsaudiobridge-client,$(HOSTOS)/$(HOSTARCH))
else
host: $(call binpath,earsaudiobridge-client,$(HOSTOS)/$(HOSTARCH)) $(call binpath,earsaudiobridge-server,$(HOSTOS)/$(HOSTARCH))
endif

# Primary linux/amd64 artifacts
$(BINDIR)/earsaudiobridge-server: FORCE
	@mkdir -p $(BINDIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(BUILDFLAGS) -ldflags '$(LDFLAGS)' -o $@ ./cmd/earsaudiobridge-server

$(BINDIR)/earsaudiobridge-client: FORCE
	@mkdir -p $(BINDIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(BUILDFLAGS) -ldflags '$(LDFLAGS)' -o $@ ./cmd/earsaudiobridge-client

# Primary windows/amd64 artifacts
$(BINDIR)/earsaudiobridge-server.exe: FORCE
	@mkdir -p $(BINDIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(BUILDFLAGS) -ldflags '$(LDFLAGS_WINDOWS)' -o $@ ./cmd/earsaudiobridge-server

$(BINDIR)/earsaudiobridge-client.exe: FORCE
	@mkdir -p $(BINDIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(BUILDFLAGS) -ldflags '$(LDFLAGS_WINDOWS)' -o $@ ./cmd/earsaudiobridge-client

# Other platforms: builds/<cmd>_<os>_<arch>[.exe]
$(BINDIR)/%: FORCE
	@mkdir -p $(BINDIR)
	CGO_ENABLED=0 GOOS=$(word 2,$(subst _, ,$(basename $(notdir $@)))) GOARCH=$(word 3,$(subst _, ,$(basename $(notdir $@)))) $(GO) build $(BUILDFLAGS) -ldflags '$(LDFLAGS)' -o $@ ./cmd/$(word 1,$(subst _, ,$(basename $(notdir $@))))

FORCE:

vet: ## go vet for all target OSes
	CGO_ENABLED=0 $(GO) vet ./...
	CGO_ENABLED=0 GOOS=windows $(GO) vet ./...
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) vet ./...

tidy: ## go mod tidy
	$(GO) mod tidy

checksums: ## SHA256SUMS for everything in builds/
	cd $(BINDIR) && sha256sum earsaudiobridge-* > SHA256SUMS && cat SHA256SUMS

run-server: ## Run the server from source (CONFIG=server.yaml)
	CGO_ENABLED=0 $(GO) run ./cmd/earsaudiobridge-server --config $(or $(CONFIG),server.yaml)

run-client: ## Run the client from source (CONFIG=client.yaml)
	CGO_ENABLED=0 $(GO) run ./cmd/earsaudiobridge-client --config $(or $(CONFIG),client.yaml)

clean: ## Remove builds/
	rm -rf $(BINDIR)

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | sort | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'
