# Frontend dependency, build, and verification targets.

# Installing frontend dependencies remains explicit. Runtime and release
# verification consume only the locked, already prepared node_modules tree.
frontend-sync:
	cd "$(FRONTEND_ROOT)" && npm ci

frontend-unit-test:
	@test -d "$(FRONTEND_ROOT)/node_modules" || { \
		echo "missing locked frontend dependencies; run make frontend-sync" >&2; \
		exit 1; \
	}
	cd "$(FRONTEND_ROOT)" && npm test

frontend-test: frontend-unit-test frontend-build
	git diff --exit-code -- cmd/meking/internal/httpservice/frontend/dist
	@test -z "$$(git status --porcelain --untracked-files=all -- cmd/meking/internal/httpservice/frontend/dist)" || { \
		git status --short --untracked-files=all -- cmd/meking/internal/httpservice/frontend/dist; \
		exit 1; \
	}
