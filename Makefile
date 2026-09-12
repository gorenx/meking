SHELL := /bin/sh
.DEFAULT_GOAL := help

include mk/config.mk

.PHONY: help build backend-build corpus-build test test-race vet quality sqlc frontend-sync frontend-build frontend-unit-test frontend-test sidecar-build sidecar-sync sidecar-resources sidecar-test release-check tidy go-env prepare-go-cache \
	init start require-sidecar ensure-sidecar prepare-python-cache

help:
	@printf '%s\n' \
		'Meking development targets:' \
		'  make build                 Build Go, frontend, and analysis sidecar artifacts' \
		'  make test                  Run the regular Go test suite' \
		'  make quality               Run tests, race tests, and vet' \
		'  make release-check         Run deterministic release verification' \
		'  make sqlc                  Regenerate all module-owned sqlc sources' \
		'  make init <project>        Initialize a Project' \
		'  make start <project>       Start the Project HTTP service' \
		'  make frontend-sync         Install locked frontend dependencies' \
		'  make frontend-unit-test    Run frontend unit tests' \
		'  make sidecar-sync          Install the locked analysis sidecar environment'

include mk/build.mk
include mk/frontend.mk
include mk/sidecar.mk
include mk/test.mk
include mk/run.mk
