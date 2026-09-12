package assembly

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/memoria-space/meking/analysis"
)

const analysisCacheFile = "analysis-cache.sqlite"

func openAnalysis(
	ctx context.Context,
	root string,
	cacheDirectory string,
	command string,
	required []analysis.Capability,
) (
	resultSession *analysis.Session,
	resultErr error,
) {
	if len(required) == 0 {
		return nil, nil
	}
	cache, err := analysis.OpenSQLiteCache(filepath.Join(
		root,
		cacheDirectory,
		analysisCacheFile,
	))
	if err != nil {
		return nil, fmt.Errorf("open assembly analysis cache: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, cache.Close())
		}
	}()
	resultSession, err = analysis.Start(ctx, analysis.ProcessConfig{
		Command:              strings.TrimSpace(command),
		RequiredCapabilities: required,
	}, cache)
	if err != nil {
		return nil, fmt.Errorf("open assembly analysis: %w", err)
	}
	return resultSession, nil
}
