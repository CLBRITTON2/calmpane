package ui

import (
	"cmp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/calmpane/claudecode"
)

// slashMenuMaxRows is how many commands the menu under the input lists at once.
const slashMenuMaxRows = 8

// slashMenu is open from a / typed into the empty input until the input no longer holds one command name.
type slashMenu struct {
	selected int
	// openedAt is the time the history's boosts are reckoned at while the menu is open.
	openedAt time.Time
}

// slashMatch is a command the menu lists for a query.
type slashMatch struct {
	command claudecode.SlashCommand
	score   int
}

// match scores after fzf's: https://github.com/junegunn/fzf/blob/master/src/algo/algo.go
const (
	fuzzyMatch        = 16
	fuzzyConsecutive  = 4
	fuzzyGapStart     = 3
	fuzzyGapExtension = 1
	fuzzyBoundary     = 8
)

// RankedSlashCommands are the commands, and the history's commands of other names with no description, whose name
// holds query's letters in order, gaps allowed, best first: the match score plus the boost the history gives at now.
// With an empty query the boosts alone rank, and equal scores keep name order.
func RankedSlashCommands(commands []claudecode.SlashCommand, uses []SlashUse, query string, now time.Time) []slashMatch {
	boosts := SlashBoosts(uses, now)
	listed := slices.Clone(commands)
	for _, use := range uses {
		if !slices.ContainsFunc(listed, func(command claudecode.SlashCommand) bool { return command.Name == use.Name }) {
			listed = append(listed, claudecode.SlashCommand{Name: use.Name, Description: ""})
		}
	}
	byName := slices.SortedStableFunc(slices.Values(listed), func(first claudecode.SlashCommand,
		second claudecode.SlashCommand) int {
		return cmp.Compare(first.Name, second.Name)
	})
	matches := []slashMatch{}
	for _, command := range byName {
		if score, matched := FuzzyScore(command.Name, query); matched {
			matches = append(matches, slashMatch{command: command, score: score + boosts[command.Name]})
		}
	}
	slices.SortStableFunc(matches, func(higher slashMatch, lower slashMatch) int {
		return cmp.Compare(lower.score, higher.score)
	})
	return matches
}

// fuzzyLetter is a letter of a matched text, lowercase, with the bonus a match on it earns at a word start: the text's
// start, after a character other than a letter or digit, or a capital after a lowercase letter.
type fuzzyLetter struct {
	lowercase rune
	boundary  int
}

// fuzzyCell is the best match of a query's letters so far with its last letter at one position. run is the boundary
// bonus of the first letter of its run of consecutive matches, which every letter of the run earns.
type fuzzyCell struct {
	score   int
	run     int
	matched bool
}

// FuzzyScore is the score of the best match of query's letters in order in text, ignoring case, and false
// when they do not all occur. An empty query matches with 0.
func FuzzyScore(text string, query string) (int, bool) {
	word := []rune(strings.ToLower(query))
	if len(word) == 0 {
		return 0, true
	}
	letters := []fuzzyLetter{}
	previous := rune(-1)
	for _, letter := range text {
		boundary := 0
		atWordStart := previous < 0 || !(unicode.IsLetter(previous) || unicode.IsDigit(previous)) ||
			(unicode.IsLower(previous) && unicode.IsUpper(letter))
		if atWordStart {
			boundary = fuzzyBoundary
		}
		letters = append(letters, fuzzyLetter{lowercase: unicode.ToLower(letter), boundary: boundary})
		previous = letter
	}
	row := make([]fuzzyCell, len(letters))
	for at, letter := range letters {
		if letter.lowercase == word[0] {
			row[at] = fuzzyCell{score: fuzzyMatch + letter.boundary, run: letter.boundary, matched: true}
		}
	}
	for _, wanted := range word[1:] {
		nextRow := make([]fuzzyCell, len(letters))
		// gapped is the best score reaching this position across a gap of at least one letter.
		gapped := fuzzyCell{}
		before := fuzzyCell{}
		for at, letter := range letters {
			cell := fuzzyCell{}
			if before.matched {
				run := max(before.run, letter.boundary)
				cell = fuzzyCell{score: before.score + fuzzyMatch + fuzzyConsecutive + run, run: run, matched: true}
			}
			if gapped.matched {
				jumped := fuzzyCell{score: gapped.score + fuzzyMatch + letter.boundary, run: letter.boundary, matched: true}
				if !cell.matched || jumped.score > cell.score || (jumped.score == cell.score && jumped.run > cell.run) {
					cell = jumped
				}
			}
			if letter.lowercase == wanted {
				nextRow[at] = cell
			}
			if gapped.matched {
				gapped.score -= fuzzyGapExtension
			}
			if before.matched && (!gapped.matched || before.score-fuzzyGapStart > gapped.score) {
				gapped = fuzzyCell{score: before.score - fuzzyGapStart, matched: true}
			}
			before = row[at]
		}
		row = nextRow
	}
	best := fuzzyCell{}
	for _, cell := range row {
		if cell.matched && (!best.matched || cell.score > best.score) {
			best = cell
		}
	}
	return best.score, best.matched
}

// slashMenuRows lists the matches from the selected one's page, the name in a column as wide as the widest shown, then
// the description cut at the view's width.
func slashMenuRows(matches []slashMatch, selected int, width int, appearance appearance) []string {
	first := max(selected-slashMenuMaxRows+1, 0)
	shown := matches[first:min(first+slashMenuMaxRows, len(matches))]
	labels := []string{}
	labelWidth := 0
	for _, match := range shown {
		label := "/" + match.command.Name
		labels = append(labels, label)
		labelWidth = max(labelWidth, ansi.StringWidth(label))
	}
	// three columns of indent, two between the columns and one of margin on the right
	descriptionWidth := max(width-3-labelWidth-2-1, 0)
	rows := []string{}
	for index, match := range shown {
		nameStyle := appearance.menuNameStyle
		if first+index == selected {
			nameStyle = appearance.menuPickedNameStyle
		}
		padding := strings.Repeat(" ", labelWidth-ansi.StringWidth(labels[index])+2)
		description := ansi.Truncate(match.command.Description, descriptionWidth, "…")
		rows = append(rows, "   "+nameStyle.Render(labels[index])+padding+appearance.workingStyle.Render(description))
	}
	return rows
}
