package community

import "testing"

func TestDefaultCommunityConfigurationsBelongToCommunity(t *testing.T) {
	detection := DefaultDetectConfig()
	if detection.MaxClusterSize != DefaultMaxClusterSize ||
		detection.Seed != DefaultCommunitySeed ||
		!detection.UseLargestConnectedComponent {
		t.Fatalf("detection defaults = %#v", detection)
	}
	if err := detection.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
