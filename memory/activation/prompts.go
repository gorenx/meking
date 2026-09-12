package activation

import _ "embed"

//go:embed prompts/recall.zh-CN.md
var recallChinese string

//go:embed prompts/recall.en.md
var recallEnglish string

// DefaultPrompts returns a fresh set of domain-owned evaluation assets for
// Project initialization. An omitted override uses these same embedded texts.
func DefaultPrompts() map[string]string {
	return map[string]string{"recall.zh-CN.md": recallChinese, "recall.en.md": recallEnglish}
}
