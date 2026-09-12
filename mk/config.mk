# Toolchain, cache, Project, and command configuration.

GO ?= go
SQLC ?= sqlc
SQLC_VERSION ?= v1.31.1
SQLC_CONFIGS := $(shell find . -type f -name sqlc.yaml \
	-not -path './.cache/*' \
	-not -path './cmd/meking/internal/httpservice/frontend/node_modules/*' | sort)
# CURDIR is provided by make and resolves to the directory where it is running.
GO_CACHE_ROOT ?= $(CURDIR)/.cache/go
GO_GOPATH := $(GO_CACHE_ROOT)/gopath
GO_MOD_CACHE := $(GO_GOPATH)/pkg/mod
GO_BUILD_CACHE := $(GO_CACHE_ROOT)/build

PYTHON_CACHE_ROOT ?= $(CURDIR)/.cache/python
UV_CACHE_ROOT ?= $(CURDIR)/.cache/uv
ANALYSIS_SIDECAR_ROOT ?= $(CURDIR)/analysis-sidecar
ANALYSIS_SIDECAR_PYTHON ?= $(ANALYSIS_SIDECAR_ROOT)/.venv/bin/python
ANALYSIS_SIDECAR_COMMAND ?= $(ANALYSIS_SIDECAR_ROOT)/.venv/bin/meking-analysis-sidecar
ANALYSIS_SIDECAR_NLTK_DATA ?= $(ANALYSIS_SIDECAR_ROOT)/.venv/nltk_data
FRONTEND_ROOT ?= $(CURDIR)/cmd/meking/internal/httpservice/frontend
WORKDIR ?=
START_ROOT ?=
ifneq ($(strip $(WORKDIR)),)
START_ROOT := $(WORKDIR)
endif
START_ADDRESS ?= 127.0.0.1
START_PORT ?= 8080
START_ANALYSIS_COMMAND ?= $(ANALYSIS_SIDECAR_COMMAND)
# Allow invoking as `make init <project>` or `make start <project>` while
# keeping START_ROOT as the single source of truth.
PROJECT_TARGET_ARG := $(word 2, $(MAKECMDGOALS))
ifneq ($(strip $(filter init start,$(MAKECMDGOALS))),)
ifneq ($(strip $(PROJECT_TARGET_ARG)),)
ifeq ($(strip $(START_ROOT)),)
START_ROOT := $(PROJECT_TARGET_ARG)
endif
# The positional Project arg is a convenience token; make it a no-op goal so
# `make init <project>` / `make start <project>` only execute the target logic.
.PHONY: $(PROJECT_TARGET_ARG)
$(PROJECT_TARGET_ARG):
	@:
endif
endif
INIT_MODEL_BASE_URL ?=
INIT_EMBEDDING_BASE_URL ?=

# Keep the downloaded toolchain, modules, checksum state, and build artifacts
# inside the project cache so sandboxed commands never write to the user GOPATH.
GO_COMMAND = env \
	GOENV=off \
	GOPATH="$(GO_GOPATH)" \
	GOMODCACHE="$(GO_MOD_CACHE)" \
	GOCACHE="$(GO_BUILD_CACHE)" \
	GOTOOLCHAIN=auto \
	$(GO)

prepare-go-cache:
	@mkdir -p "$(GO_GOPATH)" "$(GO_MOD_CACHE)" "$(GO_BUILD_CACHE)"

prepare-python-cache:
	@mkdir -p "$(PYTHON_CACHE_ROOT)" "$(UV_CACHE_ROOT)"
