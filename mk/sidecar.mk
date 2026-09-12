# Analysis sidecar environment, resource, and verification targets.

# Dependency installation is explicit and may access the package registry.
# Runtime and deterministic checks use only this locked, already prepared env.
sidecar-sync: prepare-python-cache
	env UV_CACHE_DIR="$(UV_CACHE_ROOT)" XDG_CACHE_HOME="$(PYTHON_CACHE_ROOT)" \
		uv sync --frozen --project "$(ANALYSIS_SIDECAR_ROOT)"

# NLP corpora are an explicit installation step. The sidecar only reads this
# virtual-environment-local directory and never downloads resources at runtime.
sidecar-resources:
	@test -x "$(ANALYSIS_SIDECAR_PYTHON)" || { \
		echo "missing locked analysis sidecar environment; run make sidecar-sync" >&2; \
		exit 1; \
	}
	"$(ANALYSIS_SIDECAR_PYTHON)" "$(ANALYSIS_SIDECAR_ROOT)/scripts/install_resources.py" \
		--destination "$(ANALYSIS_SIDECAR_NLTK_DATA)"

sidecar-test: prepare-python-cache
	env UV_CACHE_DIR="$(UV_CACHE_ROOT)" uv lock --check --offline \
		--project "$(ANALYSIS_SIDECAR_ROOT)"
	@test -x "$(ANALYSIS_SIDECAR_PYTHON)" || { \
		echo "missing locked analysis sidecar environment; run make sidecar-sync" >&2; \
		exit 1; \
	}
	@test -x "$(ANALYSIS_SIDECAR_COMMAND)" || { \
		echo "missing analysis sidecar executable: $(ANALYSIS_SIDECAR_COMMAND)" >&2; \
		exit 1; \
	}
	@for resource in \
		taggers/averaged_perceptron_tagger_eng \
		corpora/brown \
		tokenizers/punkt \
		tokenizers/punkt_tab \
		corpora/treebank; do \
		test -d "$(ANALYSIS_SIDECAR_NLTK_DATA)/$$resource" || { \
			echo "missing pinned NLTK resource $$resource; run make sidecar-resources" >&2; \
			exit 1; \
		}; \
	done
	env PYTHONDONTWRITEBYTECODE=1 XDG_CACHE_HOME="$(PYTHON_CACHE_ROOT)" \
		"$(ANALYSIS_SIDECAR_PYTHON)" -m pytest "$(ANALYSIS_SIDECAR_ROOT)/tests"
	env MEKING_ANALYSIS_INTEGRATION=1 \
		MEKING_ANALYSIS_COMMAND="$(ANALYSIS_SIDECAR_COMMAND)" \
		$(GO_COMMAND) test ./analysis ./assembly \
		-run 'Test(PythonSidecarProcessContract|ProjectAnalysisSidecarComposition)' -count=1
