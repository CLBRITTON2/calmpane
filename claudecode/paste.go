package claudecode

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Claude Code's limits for a paste it keeps inline in its input: a longer paste shows as its label.
const (
	maxInlinePasteChars      = 800
	maxInlinePasteLineBreaks = 2
)

// NormalizedPaste trims the paste, so it still matches the prompt Claude records with its ends trimmed.
func NormalizedPaste(content string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n"))
}

func IsShownAsLabel(paste string) bool {
	return utf8.RuneCountInString(paste) > maxInlinePasteChars || strings.Count(paste, "\n") > maxInlinePasteLineBreaks
}

// PasteLabel is the label Claude Code shows for a paste, counting its line breaks as lines.
func PasteLabel(number int, paste string) string {
	lineBreaks := strings.Count(paste, "\n")
	if lineBreaks == 0 {
		return fmt.Sprintf("[Pasted text #%d]", number)
	}
	return fmt.Sprintf("[Pasted text #%d +%d lines]", number, lineBreaks)
}

var pasteLabelPattern = regexp.MustCompile(`\[Pasted text #\d+( \+\d+ lines)?\]`)

// HoldsPasteLabel reports whether text holds a label in the form PasteLabel writes, which stands for a paste the text
// itself does not hold.
func HoldsPasteLabel(text string) bool {
	return pasteLabelPattern.MatchString(text)
}

// TextWithPastesCollapsed replaces the first occurrence of each paste, numbered from 1, with its label.
func TextWithPastesCollapsed(text string, pastes []string) string {
	for index, paste := range pastes {
		text = strings.Replace(text, paste, PasteLabel(index+1, paste), 1)
	}
	return text
}

// TextWithPastesExpanded replaces the first occurrence of each paste's label, numbered from 1, with the paste.
func TextWithPastesExpanded(text string, pastes []string) string {
	var expanded strings.Builder
	for _, part := range TextParts(text, pastes) {
		expanded.WriteString(part.Text)
	}
	return expanded.String()
}

// TextPart is a run of typed text, or the paste a label stands for.
type TextPart struct {
	Text    string
	IsPaste bool
}

// TextParts splits text at the first occurrence of each paste's label, numbered from 1, in the order they occur. A
// paste whose label the text lacks has no part.
func TextParts(text string, pastes []string) []TextPart {
	type labelPosition struct {
		start int
		end   int
		paste string
	}
	labels := []labelPosition{}
	for index, paste := range pastes {
		label := PasteLabel(index+1, paste)
		if start := strings.Index(text, label); start >= 0 {
			labels = append(labels, labelPosition{start: start, end: start + len(label), paste: paste})
		}
	}
	slices.SortFunc(labels, func(first labelPosition, second labelPosition) int { return first.start - second.start })
	parts := []TextPart{}
	typedStart := 0
	for _, label := range labels {
		if label.start > typedStart {
			parts = append(parts, TextPart{Text: text[typedStart:label.start]})
		}
		parts = append(parts, TextPart{Text: label.paste, IsPaste: true})
		typedStart = label.end
	}
	if typedStart < len(text) {
		parts = append(parts, TextPart{Text: text[typedStart:]})
	}
	return parts
}
