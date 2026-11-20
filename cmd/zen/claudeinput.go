package main

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// claudeInputMarker starts the first row of Claude Code's input box. A prompt in the transcript above it starts with ❯
// and a plain space.
const claudeInputMarker = "❯ "

type claudeInputBox struct {
	// Draft is empty when the box holds nothing or only a dimmed placeholder.
	Draft string
	Rows  int
}

// ClaudeInputBoxOnScreen reads the box from herdr pane read --format ansi: the rows between the last two rules. pane read
// drops the space at a soft wrap, so the rows are joined with one. A box scrolled past its top shows ↑ in its top rule
// and gives no draft.
func ClaudeInputBoxOnScreen(screen string) claudeInputBox {
	rows := strings.Split(screen, "\n")
	ruleRows := []int{}
	for index, row := range rows {
		text := strings.TrimRight(ansi.Strip(row), " \r")
		if text != "" && strings.Trim(text, "─") == "" {
			ruleRows = append(ruleRows, index)
		}
	}
	if len(ruleRows) < 2 {
		return claudeInputBox{}
	}
	boxRows := rows[ruleRows[len(ruleRows)-2]+1 : ruleRows[len(ruleRows)-1]]
	if len(boxRows) == 0 || !strings.HasPrefix(ansi.Strip(boxRows[0]), claudeInputMarker) {
		return claudeInputBox{}
	}
	rowTexts := []string{}
	holdsUndimmedText := false
	// The marker is drawn undimmed above a placeholder, and after SGR sequences while Claude works.
	rowsAfterMarker := slices.Clone(boxRows)
	rowsAfterMarker[0] = strings.Replace(rowsAfterMarker[0], claudeInputMarker, "", 1)
	for _, row := range rowsAfterMarker {
		text := strings.TrimSpace(ansi.Strip(row))
		if text != "" {
			rowTexts = append(rowTexts, text)
		}
		holdsUndimmedText = holdsUndimmedText || DrawsUndimmedText(row)
	}
	if !holdsUndimmedText {
		return claudeInputBox{}
	}
	return claudeInputBox{Draft: strings.Join(rowTexts, " "), Rows: len(boxRows)}
}

// DrawsUndimmedText reports whether row draws a character other than a space without dim. herdr 0.9.5 writes each SGR
// attribute as its own sequence, so dim is \x1b[2m and its end a reset.
func DrawsUndimmedText(row string) bool {
	dimmed := false
	var state byte
	for len(row) > 0 {
		sequence, width, length, newState := ansi.DecodeSequence(row, state, nil)
		switch {
		case sequence == "\x1b[2m":
			dimmed = true
		case sequence == "\x1b[0m" || sequence == "\x1b[m" || sequence == "\x1b[22m":
			dimmed = false
		case width > 0 && !dimmed && strings.TrimSpace(sequence) != "":
			return true
		}
		state = newState
		row = row[length:]
	}
	return false
}
