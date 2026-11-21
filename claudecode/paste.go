package claudecode

import (
	"fmt"
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

// TextWithPastesCollapsed replaces the first occurrence of each paste, numbered from 1, with its label.
func TextWithPastesCollapsed(text string, pastes []string) string {
	for index, paste := range pastes {
		text = strings.Replace(text, paste, PasteLabel(index+1, paste), 1)
	}
	return text
}

// TextWithPastesExpanded replaces the first occurrence of each paste's label, numbered from 1, with the paste.
func TextWithPastesExpanded(text string, pastes []string) string {
	for index, paste := range pastes {
		text = strings.Replace(text, PasteLabel(index+1, paste), paste, 1)
	}
	return text
}
