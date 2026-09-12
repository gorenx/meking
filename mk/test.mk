# Go quality and release targets.

GO_TEST_FLAGS ?=

test: prepare-go-cache
	$(GO_COMMAND) test $(GO_TEST_FLAGS) ./...

test-race: prepare-go-cache
	$(GO_COMMAND) test -race $(GO_TEST_FLAGS) ./...

vet: prepare-go-cache
	$(GO_COMMAND) vet ./...

quality: test test-race vet

.PHONY: test-mas test-activation mas-reference

test-activation: prepare-go-cache
	$(GO_COMMAND) test -count=1 $(GO_TEST_FLAGS) ./memory/activation/... ./mcp ./project ./assembly

test-mas: prepare-go-cache
	$(GO_COMMAND) test -count=1 ./knowledge/mas

mas-reference: prepare-python-cache
	UV_CACHE_DIR="$(UV_CACHE_ROOT)" uv run --no-project --with fsrs==6.3.2 python knowledge/mas/testdata/generate_reference.py

# release-check is the deterministic project gate. Target-scale performance
# thresholds remain explicit release operations.
release-check: quality sidecar-test frontend-test
