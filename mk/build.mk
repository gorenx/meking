# Source generation and application build targets.

build: backend-build frontend-build sidecar-build

backend-build: prepare-go-cache
	@mkdir -p "$(CURDIR)/bin"
	$(GO_COMMAND) build ./...
	$(GO_COMMAND) build -o "$(CURDIR)/bin/meking" ./cmd/meking

corpus-build: prepare-go-cache
	$(GO_COMMAND) build ./corpus/...

frontend-build: prepare-go-cache
	@test -d "$(FRONTEND_ROOT)/node_modules" || { \
		echo "missing frontend dependencies; run make frontend-sync" >&2; \
		exit 1; \
	}
	cd "$(FRONTEND_ROOT)" && npm run build

sidecar-build: prepare-python-cache
	@if ! [ -x "$(ANALYSIS_SIDECAR_COMMAND)" ]; then \
		$(MAKE) sidecar-sync; \
	fi
	@test -x "$(ANALYSIS_SIDECAR_COMMAND)" || { \
		echo "missing analysis sidecar executable: $(ANALYSIS_SIDECAR_COMMAND)" >&2; \
		echo "请先执行 make sidecar-sync" >&2; \
		exit 1; \
	}

# sqlc is an explicit source-generation step. Each persistence adapter owns its
# config below its module directory; this root target only discovers and runs
# those configs. Generated files are committed, so regular build and test
# targets never install or download tools implicitly.
sqlc:
	@command -v "$(SQLC)" >/dev/null 2>&1 || { \
		echo "missing sqlc $(SQLC_VERSION)" >&2; \
		exit 1; \
	}
	@test "$$($(SQLC) version)" = "$(SQLC_VERSION)" || { \
		echo "sqlc version mismatch: expected $(SQLC_VERSION), got $$($(SQLC) version)" >&2; \
		exit 1; \
	}
	@test -n "$(SQLC_CONFIGS)" || { \
		echo "no module-owned sqlc.yaml files found" >&2; \
		exit 1; \
	}
	@for config in $(SQLC_CONFIGS); do \
		echo "$(SQLC) generate -f $$config"; \
		$(SQLC) generate -f "$$config" || exit 1; \
	done

tidy: prepare-go-cache
	$(GO_COMMAND) mod tidy

go-env: prepare-go-cache
	$(GO_COMMAND) env GOVERSION GOPATH GOMODCACHE GOCACHE GOTOOLCHAIN GOSUMDB
