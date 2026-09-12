# Project initialization and Web service runtime targets.

# Initialize one Project with defaults.
init:
	@test -n "$(strip $(START_ROOT))" || { \
		echo "make init requires Project path. Set START_ROOT=<path-to-project> (or WORKDIR=<path-to-project>)."; \
		exit 1; \
	}
	$(GO_COMMAND) run ./cmd/meking init "$(START_ROOT)" \
		$(if $(strip $(INIT_MODEL_BASE_URL)),--model-base-url "$(INIT_MODEL_BASE_URL)",) \
		$(if $(strip $(INIT_EMBEDDING_BASE_URL)),--embedding-base-url "$(INIT_EMBEDDING_BASE_URL)",)

# Start the read/query Web service for one Project and its Zones.
start: prepare-go-cache frontend-build
	@test -n "$(strip $(START_ROOT))" || { \
		echo "make start requires Project path. Set START_ROOT=<path-to-project> (or WORKDIR=<path-to-project>)."; \
		exit 1; \
	}
	$(GO_COMMAND) run ./cmd/meking start \
		--root "$(START_ROOT)" \
		--analysis-command "$(START_ANALYSIS_COMMAND)" \
		--address "$(START_ADDRESS)" \
		--port "$(START_PORT)"

ensure-sidecar:
	@echo "syncing analysis sidecar environment..."
	$(MAKE) sidecar-sync
	@test -x "$(START_ANALYSIS_COMMAND)" || { \
		echo "missing analysis sidecar executable: $(START_ANALYSIS_COMMAND)"; \
		exit 1; \
	}

require-sidecar:
	@test -x "$(START_ANALYSIS_COMMAND)" || { \
		echo "missing analysis sidecar: $(START_ANALYSIS_COMMAND)"; \
		echo "请先执行 make sidecar-sync"; \
		exit 1; \
	}
