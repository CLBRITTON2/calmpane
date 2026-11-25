package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Run("exampleParsesToRosePineWithStatusLineDirUnderProfile", exampleParsesToRosePineWithStatusLineDirUnderProfile)
	t.Run("missingFileIsWrittenWithExampleThenRead", missingFileIsWrittenWithExampleThenRead)
	t.Run("unknownFieldFailsNamingFieldAndLine", unknownFieldFailsNamingFieldAndLine)
	t.Run("missingFieldFailsNamingField", missingFieldFailsNamingField)
	t.Run("badColorFailsNamingFieldAndValue", badColorFailsNamingFieldAndValue)
	t.Run("unknownThemeFailsNamingFieldAndValue", unknownThemeFailsNamingFieldAndValue)
	t.Run("wideLineCharacterFailsNamingFieldAndValue", wideLineCharacterFailsNamingFieldAndValue)
	t.Run("badCommentStyleFailsNamingFieldAndValue", badCommentStyleFailsNamingFieldAndValue)
	t.Run("emptyClaudeScreenKeyFailsNamingField", emptyClaudeScreenKeyFailsNamingField)
	t.Run("missingExpandPasteKeyFailsNamingField", missingExpandPasteKeyFailsNamingField)
	t.Run("expandPasteKeyNotWrittenAsBubbleteaKeyFailsNamingFieldAndValue",
		expandPasteKeyNotWrittenAsBubbleteaKeyFailsNamingFieldAndValue)
	t.Run("expandPasteKeyWrittenAsBubbleteaKeyParses", expandPasteKeyWrittenAsBubbleteaKeyParses)
	t.Run("emptyStatusLineDirStaysEmpty", emptyStatusLineDirStaysEmpty)
}

func exampleParsesToRosePineWithStatusLineDirUnderProfile(t *testing.T) {
	profile, err := os.UserHomeDir()
	require.NoError(t, err)
	want := Config{
		Colors: Colors{
			PromptBand: "#26233a", PromptMarker: "#c4a7e7", PromptText: "#e0def4", PromptNote: "#6e6a86",
			ReplyText: "#e0def4", Heading: "#c4a7e7", InlineCode: "#f6c177", Link: "#9ccfd8", Strong: "#e0def4",
			Question: "#f6c177", Option: "#908caa", Working: "#6e6a86", Waiting: "#f6c177", Done: "#9ccfd8", Error: "#eb6f92",
			Separator: "#403d52", CodeRule: "#403d52", CodeLabel: "#6e6a86",
		},
		Lines:      Lines{Separator: "─", CodeRule: "─"},
		Code:       Code{Theme: "rose-pine", Comment: "bold #c4a7e7"},
		StatusLine: StatusLine{Dir: filepath.Join(profile, `AppData\Local\windows-dev\statusline`)},
		Keys:       Keys{ClaudeScreen: "alt+x", ExpandPaste: "ctrl+e"},
	}
	got, err := Example()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func missingFileIsWrittenWithExampleThenRead(t *testing.T) {
	file := filepath.Join(t.TempDir(), "calmpane", "config.toml")
	got, err := Load(file)
	require.NoError(t, err)
	written, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, string(exampleDocument), string(written))
	want, err := Example()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// exampleWith is the example document with its line old replaced by replacement.
func exampleWith(t *testing.T, old string, replacement string) []byte {
	document := string(exampleDocument)
	require.Contains(t, document, old)
	replaced := strings.Replace(document, old, replacement, 1)
	return []byte(replaced)
}

func assertParseFailsWith(t *testing.T, document []byte, wantParts []string) {
	_, err := Parse(document)
	require.Error(t, err)
	for _, part := range wantParts {
		require.ErrorContains(t, err, part)
	}
}

func unknownFieldFailsNamingFieldAndLine(t *testing.T) {
	document := exampleWith(t, `heading = "#c4a7e7"`, "heading = \"#c4a7e7\"\nheadline = \"#ffffff\"")
	assertParseFailsWith(t, document, []string{"unknown field colors.headline", `headline = "#ffffff"`})
}

func missingFieldFailsNamingField(t *testing.T) {
	document := exampleWith(t, `inline_code = "#f6c177"`, "")
	assertParseFailsWith(t, document, []string{"missing field colors.inline_code"})
}

func badColorFailsNamingFieldAndValue(t *testing.T) {
	document := exampleWith(t, `heading = "#c4a7e7"`, `heading = "c4a7e7"`)
	assertParseFailsWith(t, document, []string{`colors.heading = "c4a7e7"`})
}

func unknownThemeFailsNamingFieldAndValue(t *testing.T) {
	document := exampleWith(t, `theme = "rose-pine"`, `theme = "rose-pink"`)
	assertParseFailsWith(t, document, []string{`code.theme = "rose-pink"`})
}

func wideLineCharacterFailsNamingFieldAndValue(t *testing.T) {
	document := exampleWith(t, `separator = "─"`, `separator = "──"`)
	assertParseFailsWith(t, document, []string{`lines.separator = "──"`})
}

func badCommentStyleFailsNamingFieldAndValue(t *testing.T) {
	document := exampleWith(t, `comment = "bold #c4a7e7"`, `comment = "heavy #c4a7e7"`)
	assertParseFailsWith(t, document, []string{`code.comment = "heavy #c4a7e7"`})
}

func emptyClaudeScreenKeyFailsNamingField(t *testing.T) {
	document := exampleWith(t, `claude_screen = "alt+x"`, `claude_screen = ""`)
	assertParseFailsWith(t, document, []string{`keys.claude_screen = ""`})
}

func missingExpandPasteKeyFailsNamingField(t *testing.T) {
	document := exampleWith(t, `expand_paste = "ctrl+e"`, "")
	assertParseFailsWith(t, document, []string{"missing field keys.expand_paste"})
}

func expandPasteKeyNotWrittenAsBubbleteaKeyFailsNamingFieldAndValue(t *testing.T) {
	for _, key := range []string{"", "ctrl+", "control+e", "shift+ctrl+e", "ctrl+ctrl+e", "ctrl+enterr", "ctrl+ee", " "} {
		document := exampleWith(t, `expand_paste = "ctrl+e"`, `expand_paste = "`+key+`"`)
		assertParseFailsWith(t, document, []string{`keys.expand_paste = "` + key + `" is not a key name`})
	}
}

func expandPasteKeyWrittenAsBubbleteaKeyParses(t *testing.T) {
	for _, key := range []string{"ctrl+o", "alt+enter", "f5", "ctrl+shift+up", "ctrl+space", "?"} {
		document := exampleWith(t, `expand_paste = "ctrl+e"`, `expand_paste = "`+key+`"`)
		got, err := Parse(document)
		require.NoError(t, err)
		require.Equal(t, key, got.Keys.ExpandPaste)
	}
}

func emptyStatusLineDirStaysEmpty(t *testing.T) {
	document := exampleWith(t, `dir = '~\AppData\Local\windows-dev\statusline'`, `dir = ""`)
	got, err := Parse(document)
	require.NoError(t, err)
	require.Empty(t, got.StatusLine.Dir)
}
