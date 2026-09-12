package report

import (
	"fmt"
	"strings"
)

const maximumFragmentWords = 250

const reportFragmentPrompt = `You summarize one evidence batch from a larger graph Community.

Write a concise grounded fragment. Preserve the supplied dataset names and human_readable_id values in [Data: ...] references. Do not invent facts, a Community title, or an impact rating. Return only the structured fragment requested by the response schema. Limit content to %d words.

# Evidence batch

%s`

const reportFragmentMergePrompt = `You combine ordered grounded fragments from one graph Community.

Remove repetition while preserving every distinct supported finding and its original [Data: ...] references. Do not invent facts, a Community title, or an impact rating. Return only the structured fragment requested by the response schema. Limit content to %d words.

# Ordered fragments

%s`

func renderFragmentPrompt(context string, maxWords int) string {
	return fmt.Sprintf(reportFragmentPrompt, maxWords, context)
}

func renderFragmentMergePrompt(fragments []string, maxWords int) string {
	return fmt.Sprintf(
		reportFragmentMergePrompt,
		maxWords,
		renderFragments(fragments),
	)
}

func renderFragments(fragments []string) string {
	var result strings.Builder
	for index, fragment := range fragments {
		if index > 0 {
			result.WriteString("\n\n")
		}
		fmt.Fprintf(&result, "-----Fragment %d-----\n%s", index, fragment)
	}
	return result.String()
}

func fragmentWordLimit(config Config) int {
	// Intermediate output is deliberately shorter than a publishable Report so
	// several Fragments can normally fit the next merge round. Smaller configured
	// reports keep their own stricter word limit.
	if config.MaxReportLength < maximumFragmentWords {
		return config.MaxReportLength
	}
	return maximumFragmentWords
}
