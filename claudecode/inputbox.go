package claudecode

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// claudeInputMarker starts the first row of Claude Code's input box. A prompt in the transcript above it starts with ❯
// and a plain space.
const claudeInputMarker = "❯ "

type InputBox struct {
	// Draft is empty when the box holds nothing or only a dimmed placeholder.
	Draft string
	Rows  int
}

// InputBoxOnScreen reads the box from a screen captured with its SGR sequences: the rows between the last two rules. A
// rule starts and ends with ─ and can carry text inside: the session name a /rename gave, or ↑ in the top rule of a box
// scrolled past its top, which gives no draft. A capture drops the space at a soft wrap, so the rows are joined with
// one.
func InputBoxOnScreen(screen string) InputBox {
	boxRows := inputBoxRows(screen)
	if boxRows == nil {
		return InputBox{}
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
		holdsUndimmedText = holdsUndimmedText || drawsUndimmedText(row)
	}
	if !holdsUndimmedText {
		return InputBox{}
	}
	return InputBox{Draft: strings.Join(rowTexts, " "), Rows: len(boxRows)}
}

// InputBoxShown reports whether the screen shows Claude Code's input box, which a picker or dialog such as /model's
// takes the place of.
func InputBoxShown(screen string) bool {
	return inputBoxRows(screen) != nil
}

// inputBoxRows holds the rows between the last two rules, nil unless the first of them starts with the input marker.
func inputBoxRows(screen string) []string {
	rows := strings.Split(screen, "\n")
	ruleRows := []int{}
	for index, row := range rows {
		text := strings.TrimRight(ansi.Strip(row), " \r")
		if strings.HasPrefix(text, "─") && strings.HasSuffix(text, "─") {
			ruleRows = append(ruleRows, index)
		}
	}
	if len(ruleRows) < 2 {
		return nil
	}
	boxRows := rows[ruleRows[len(ruleRows)-2]+1 : ruleRows[len(ruleRows)-1]]
	if len(boxRows) == 0 || !strings.HasPrefix(ansi.Strip(boxRows[0]), claudeInputMarker) {
		return nil
	}
	return boxRows
}

// drawsUndimmedText reports whether row draws a character other than a space without dim. herdr 0.9.5 writes each SGR
// attribute as its own sequence, so dim is \x1b[2m and its end a reset.
func drawsUndimmedText(row string) bool {
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
