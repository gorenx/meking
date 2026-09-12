package community

const (
	// CurrentDetectorVersion identifies the Community detection contract formed
	// by graph normalization, LCC selection, hierarchical Leiden options, and
	// membership collection. Increment it only when those rules change in a way
	// that can change Community membership for the same graph and DetectConfig.
	CurrentDetectorVersion uint32 = 1
	// DefaultMaxClusterSize is the product's recursive community split limit.
	DefaultMaxClusterSize = 10
	// DefaultCommunitySeed keeps hierarchical clustering deterministic.
	DefaultCommunitySeed int64 = 0xDEADBEEF
	// DefaultRelationChangeThreshold reruns Community detection after this many
	// committed Relation creations, updates, or deletions have accumulated.
	DefaultRelationChangeThreshold uint64 = 5
	// DefaultReportMaxLength is the requested report word limit.
	DefaultReportMaxLength = 2000
	// DefaultReportMaxInputTokens bounds source context for one report.
	DefaultReportMaxInputTokens = 8000
)

// DefaultDetectConfig returns the community-owned product clustering policy.
func DefaultDetectConfig() DetectConfig {
	return DetectConfig{
		MaxClusterSize:               DefaultMaxClusterSize,
		UseLargestConnectedComponent: true,
		Seed:                         DefaultCommunitySeed,
	}
}

// Validate checks community detection decisions before graph processing.
func (c DetectConfig) Validate() error {
	if c.MaxClusterSize <= 0 {
		return ErrInvalidMaxClusterSize
	}
	return nil
}
