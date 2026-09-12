package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const moduleImportPath = "github.com/memoria-space/meking/"

// TestProductionPackageDependencies keeps domain packages independent from
// orchestration and adapters as the modular monolith grows. The allowlist is
// intentionally expressed at top-level package granularity because nested
// packages, such as project/prompts, remain owned by their top-level module.
func TestProductionPackageDependencies(t *testing.T) {
	allowed := map[string]map[string]bool{
		"analysis": {
			"analysis": true,
		},
		"cmd": {
			"assembly":     true,
			"cmd":          true,
			"controlplane": true,
			"corpus":       true,
			"journal":      true,
			"mcp":          true,
			"project":      true,
			"query":        true,
			"zone":         true,
		},
		"community": {
			"community": true,
			"internal":  true,
			"knowledge": true,
			"zone":      true,
		},
		"controlplane": {
			"controlplane": true,
			"transaction":  true,
			"zone":         true,
		},
		"corpus": {
			"corpus":   true,
			"internal": true,
			"zone":     true,
		},
		"epoch": {
			"epoch":     true,
			"internal":  true,
			"knowledge": true,
			"zone":      true,
		},
		"internal": {
			"journal": true,
			"zone":    true,
		},
		"knowledge": {
			"internal":    true,
			"knowledge":   true,
			"transaction": true,
			"zone":        true,
		},
		"journal": {
			"journal": true,
			"zone":    true,
		},
		"memory": {
			"corpus":    true,
			"knowledge": true,
			"memory":    true,
		},
		"mcp": {
			"corpus":    true,
			"knowledge": true,
			"memory":    true,
			"project":   true,
			"zone":      true,
		},
		"agent": {
			"agent": true,
		},
		"project": {
			"analysis":  true,
			"community": true,
			"corpus":    true,
			"internal":  true,
			"knowledge": true,
			"project":   true,
			"query":     true,
			"semantic":  true,
		},
		"query": {
			"knowledge": true,
			"query":     true,
		},
		"semantic": {
			"semantic": true,
		},
		"transaction": {},
		"assembly": {
			"analysis":     true,
			"community":    true,
			"controlplane": true,
			"corpus":       true,
			"epoch":        true,
			"internal":     true,
			"journal":      true,
			"knowledge":    true,
			"memory":       true,
			"mcp":          true,
			"agent":        true,
			"project":      true,
			"query":        true,
			"semantic":     true,
			"transaction":  true,
			"zone":         true,
		},
		"zone": {
			"internal": true,
			"zone":     true,
		},
	}
	productionPackages, err := discoverProductionTopLevelPackages()
	if err != nil {
		t.Fatalf("discover production packages: %v", err)
	}
	for owner := range productionPackages {
		if _, registered := allowed[owner]; !registered {
			t.Errorf("production package %q is missing from the architecture dependency graph", owner)
		}
	}

	for owner, dependencies := range allowed {
		owner := owner
		dependencies := dependencies
		t.Run(owner, func(t *testing.T) {
			err := filepath.WalkDir(owner, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return nil
				}

				file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
				if err != nil {
					return err
				}
				for _, imported := range file.Imports {
					dependencyPath, local, err := localPackagePath(imported)
					if err != nil {
						return err
					}
					dependency, _, _ := strings.Cut(dependencyPath, "/")
					if local && !dependencies[dependency] &&
						!allowedConsumerAdapterDependency(owner, path, dependencyPath) {
						t.Errorf("%s imports forbidden local package %q", path, dependencyPath)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("inspect production package %s: %v", owner, err)
			}
		})
	}
}

// allowedConsumerAdapterDependency permits explicit integration capabilities
// and consumer adapters to map between public application contracts. Core
// packages remain unable to import provider persistence.
func allowedConsumerAdapterDependency(owner, path, dependencyPath string) bool {
	normalized := filepath.ToSlash(path)
	dependency, _, _ := strings.Cut(dependencyPath, "/")
	// Project loads activation-owned configuration; the observation application
	// owns Zone scope and the shared transaction, not Recorder or Searcher.
	if owner == "project" && dependencyPath == "memory/activation" {
		return true
	}
	if owner == "memory" && strings.HasPrefix(normalized, "memory/activation/") {
		if dependencyPath == "zone" || dependencyPath == "transaction" {
			return true
		}
		if strings.HasPrefix(normalized, "memory/activation/adapter/sqlite/") && dependencyPath == "transaction/adapter/sqlite" {
			return true
		}
	}
	if owner == "controlplane" &&
		strings.HasPrefix(normalized, "controlplane/adapter/journal/") {
		return dependency == "journal" ||
			dependencyPath == "community/integration" ||
			dependencyPath == "corpus/integration" ||
			dependencyPath == "epoch/integration" ||
			dependencyPath == "knowledge/integration"
	}
	if owner == "knowledge" {
		if strings.HasPrefix(normalized, "knowledge/adapter/sqlite/") {
			return dependency == "journal" || dependency == "transaction"
		}
		if strings.HasPrefix(normalized, "knowledge/zonemerger/") {
			return dependency == "transaction"
		}
		if normalized == "knowledge/extraction/application.go" {
			return dependency == "journal" || dependency == "transaction"
		}
	}
	if owner == "memory" &&
		strings.HasPrefix(normalized, "memory/adapter/semantic/") {
		return dependency == "semantic"
	}
	if owner == "corpus" {
		switch normalized {
		case "corpus/textunits/vectorindex/application.go":
			return dependency == "semantic"
		}
		if strings.HasPrefix(normalized, "corpus/zonemerger/adapter/zone/") {
			return dependency == "zone"
		}
	}
	if owner == "community" {
		switch {
		case normalized == "community/structurebuild/builder.go":
			return dependency == "transaction"
		case normalized == "community/reportgeneration/builder.go":
			return dependency == "transaction"
		case normalized == "community/adapter/sqlite/write.go":
			return dependency == "transaction"
		case normalized == "community/adapter/journal/structure_derivation_action.go":
			return dependency == "journal" || dependencyPath == "knowledge/integration"
		case normalized == "community/adapter/journal/report_generation_action.go":
			return dependency == "journal" || dependencyPath == "epoch/integration"
		case strings.HasPrefix(normalized, "community/adapter/knowledge/"):
			return dependencyPath == "knowledge"
		case strings.HasPrefix(normalized, "community/adapter/corpus/"):
			return dependencyPath == "corpus" || dependencyPath == "corpus/textunits"
		case strings.HasPrefix(normalized, "community/adapter/agent/"):
			return dependencyPath == "agent"
		}
	}
	if owner == "zone" {
		switch {
		case normalized == "zone/merger/application.go":
			return dependency == "transaction"
		case strings.HasPrefix(normalized, "zone/merger/adapter/journal/"):
			return dependency == "journal" || dependencyPath == "epoch/integration"
		case strings.HasPrefix(normalized, "zone/merger/adapter/sqlite/"):
			return dependency == "transaction"
		case strings.HasPrefix(normalized, "zone/merger/adapter/epoch/"):
			return dependency == "epoch" || dependency == "journal"
		}
	}
	if owner == "semantic" {
		switch {
		case strings.HasPrefix(normalized, "semantic/adapter/sqlite/"):
			return dependency == "zone"
		case strings.HasPrefix(normalized, "semantic/adapter/agent/"):
			return dependencyPath == "agent"
		}
	}
	if owner == "epoch" {
		switch {
		case strings.HasPrefix(normalized, "epoch/lifecycle/"):
			return dependency == "journal" || dependency == "transaction"
		case strings.HasPrefix(normalized, "epoch/adapter/journal/"):
			return dependency == "journal" ||
				dependencyPath == "community/integration" ||
				dependencyPath == "knowledge/integration"
		case strings.HasPrefix(normalized, "epoch/adapter/sqlite/"):
			return dependency == "transaction"
		case strings.HasPrefix(normalized, "epoch/adapter/readiness/"):
			return dependency == "community" ||
				dependency == "corpus" ||
				dependency == "knowledge"
		}
	}
	if dependency == "semantic" {
		switch owner {
		case "community":
			if strings.HasPrefix(normalized, "community/report/vectorindex/") {
				return true
			}
		case "corpus":
			if strings.HasPrefix(normalized, "corpus/textunits/vectorindex/") {
				return true
			}
		case "knowledge":
			if strings.HasPrefix(normalized, "knowledge/vectorindex/") {
				return true
			}
		}
	}
	if !strings.Contains(normalized, "/adapter/") {
		return false
	}
	if dependency == "journal" && strings.Contains(normalized, "/adapter/journal/") {
		switch owner {
		case "community", "corpus", "epoch", "knowledge":
			return true
		}
	}
	if dependency == "transaction" && strings.Contains(normalized, "/adapter/sqlite/") {
		return owner == "corpus" || owner == "journal" || owner == "project"
	}
	switch owner {
	case "community":
		return false
	case "corpus":
		if strings.Contains(normalized, "/adapter/journal/") && dependencyPath == "knowledge/integration" {
			return true
		}
		return strings.Contains(normalized, "/adapter/analysis/") && dependency == "analysis"
	case "query":
		return publicApplicationPackage(dependencyPath) &&
			(dependency == "community" || dependency == "corpus" || dependency == "epoch" ||
				dependency == "knowledge" || dependency == "agent" || dependency == "semantic")
	case "knowledge":
		return publicApplicationPackage(dependencyPath) &&
			(dependency == "corpus" || dependency == "knowledge" || dependency == "agent" ||
				dependency == "project")
	}
	return false
}

func publicApplicationPackage(path string) bool {
	for _, segment := range strings.Split(path, "/") {
		if segment == "adapter" || segment == "sqlite" || segment == "internal" {
			return false
		}
	}
	return true
}

func TestConsumerAdaptersCannotImportProviderPersistence(t *testing.T) {
	if allowedConsumerAdapterDependency(
		"query",
		"query/report/adapter/publication.go",
		"community/sqlite",
	) {
		t.Fatal("Query consumer adapter may not import Community SQLite")
	}
	if !allowedConsumerAdapterDependency(
		"query",
		"query/report/adapter/publication.go",
		"community",
	) {
		t.Fatal("Query consumer adapter must be able to import the Community application package")
	}
	if allowedConsumerAdapterDependency(
		"query",
		"query/global/adapter/evidence.go",
		"knowledge/mvcc/adapter",
	) {
		t.Fatal("Query consumer adapter may not depend on another domain's adapter")
	}
	if !allowedConsumerAdapterDependency(
		"query",
		"query/drift/adapter/semantic.go",
		"semantic",
	) {
		t.Fatal("Query consumer adapter must be able to import the Semantic application package")
	}
	if allowedConsumerAdapterDependency(
		"query",
		"query/drift/adapter/local.go",
		"project",
	) {
		t.Fatal("Query consumer adapter may not depend on Project orchestration")
	}
	if !allowedConsumerAdapterDependency(
		"query",
		"query/basic/adapter/model.go",
		"agent",
	) {
		t.Fatal("Query model adapter must be able to import Agent")
	}
	if !allowedConsumerAdapterDependency(
		"knowledge",
		"knowledge/extraction/adapter/agent/completion.go",
		"agent",
	) {
		t.Fatal("Knowledge Extraction adapter must be able to import Agent")
	}
	if !allowedConsumerAdapterDependency(
		"community",
		"community/adapter/agent/report.go",
		"agent",
	) {
		t.Fatal("Community Report adapter must be able to import Agent")
	}
	if !allowedConsumerAdapterDependency(
		"semantic",
		"semantic/adapter/agent/embedder.go",
		"agent",
	) {
		t.Fatal("Semantic Embedder adapter must be able to import Agent")
	}
}

func discoverProductionTopLevelPackages() (map[string]struct{}, error) {
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}
	packages := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" {
			continue
		}
		root := entry.Name()
		err := filepath.WalkDir(root, func(path string, child fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if child.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			packages[root] = struct{}{}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return packages, nil
}

func localPackagePath(imported *ast.ImportSpec) (string, bool, error) {
	path, err := strconv.Unquote(imported.Path.Value)
	if err != nil {
		return "", false, err
	}
	path, local := strings.CutPrefix(path, moduleImportPath)
	if !local {
		return "", false, nil
	}
	return path, true, nil
}
