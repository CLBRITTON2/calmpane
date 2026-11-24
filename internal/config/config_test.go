package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
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
	if err != nil {
		t.Fatalf("Example: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("example = %+v, want %+v", got, want)
	}
}

func missingFileIsWrittenWithExampleThenRead(t *testing.T) {
	file := filepath.Join(t.TempDir(), "calmpane", "config.toml")
	got, err := Load(file)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	written, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read the written config: %v", err)
	}
	if string(written) != string(exampleDocument) {
		t.Errorf("written config = %q, want the example", written)
	}
	want, err := Example()
	if err != nil {
		t.Fatalf("Example: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loaded config = %+v, want the example %+v", got, want)
	}
}

// exampleWith is the example document with its line old replaced by replacement.
func exampleWith(t *testing.T, old string, replacement string) []byte {
	document := string(exampleDocument)
	if !strings.Contains(document, old) {
		t.Fatalf("the example has no line %q", old)
	}
	return []byte(strings.Replace(document, old, replacement, 1))
}

func assertParseFailsWith(t *testing.T, document []byte, wantParts []string) {
	_, err := Parse(document)
	if err == nil {
		t.Fatalf("Parse succeeded, want an error holding %q", wantParts)
	}
	for _, part := range wantParts {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error = %q, want it to hold %q", err, part)
		}
	}
}

func unknownFieldFailsNamingFieldAndLine(t *testing.T) {
	document := exampleWith(t, `heading = "#c4a7e7"`, "heading = \"#c4a7e7\"\nheadline = \"#ffffff\"")
	assertParseFailsWith(t, document, []string{"unknown field colors.headline", `headline = "#ffffff"`})
}

func missingFieldFailsNamingField(t *testing.T) {
	assertParseFailsWith(t, exampleWith(t, `inline_code = "#f6c177"`, ""), []string{"missing field colors.inline_code"})
}

func badColorFailsNamingFieldAndValue(t *testing.T) {
	assertParseFailsWith(t, exampleWith(t, `heading = "#c4a7e7"`, `heading = "c4a7e7"`),
		[]string{`colors.heading = "c4a7e7"`})
}

func unknownThemeFailsNamingFieldAndValue(t *testing.T) {
	assertParseFailsWith(t, exampleWith(t, `theme = "rose-pine"`, `theme = "rose-pink"`),
		[]string{`code.theme = "rose-pink"`})
}

func wideLineCharacterFailsNamingFieldAndValue(t *testing.T) {
	assertParseFailsWith(t, exampleWith(t, `separator = "─"`, `separator = "──"`), []string{`lines.separator = "──"`})
}

func badCommentStyleFailsNamingFieldAndValue(t *testing.T) {
	assertParseFailsWith(t, exampleWith(t, `comment = "bold #c4a7e7"`, `comment = "heavy #c4a7e7"`),
		[]string{`code.comment = "heavy #c4a7e7"`})
}

func emptyClaudeScreenKeyFailsNamingField(t *testing.T) {
	assertParseFailsWith(t, exampleWith(t, `claude_screen = "alt+x"`, `claude_screen = ""`), []string{`keys.claude_screen = ""`})
}

func missingExpandPasteKeyFailsNamingField(t *testing.T) {
	assertParseFailsWith(t, exampleWith(t, `expand_paste = "ctrl+e"`, ""), []string{"missing field keys.expand_paste"})
}

func expandPasteKeyNotWrittenAsBubbleteaKeyFailsNamingFieldAndValue(t *testing.T) {
	for _, key := range []string{"", "ctrl+", "control+e", "shift+ctrl+e", "ctrl+ctrl+e", "ctrl+enterr", "ctrl+ee", " "} {
		assertParseFailsWith(t, exampleWith(t, `expand_paste = "ctrl+e"`, `expand_paste = "`+key+`"`),
			[]string{`keys.expand_paste = "` + key + `" is not a key name`})
	}
}

func expandPasteKeyWrittenAsBubbleteaKeyParses(t *testing.T) {
	for _, key := range []string{"ctrl+o", "alt+enter", "f5", "ctrl+shift+up", "ctrl+space", "?"} {
		got, err := Parse(exampleWith(t, `expand_paste = "ctrl+e"`, `expand_paste = "`+key+`"`))
		if err != nil {
			t.Errorf("Parse with expand_paste %q: %v", key, err)
			continue
		}
		if got.Keys.ExpandPaste != key {
			t.Errorf("keys.expand_paste = %q, want %q", got.Keys.ExpandPaste, key)
		}
	}
}

func emptyStatusLineDirStaysEmpty(t *testing.T) {
	got, err := Parse(exampleWith(t, `dir = '~\AppData\Local\windows-dev\statusline'`, `dir = ""`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.StatusLine.Dir != "" {
		t.Errorf("status_line.dir = %q, want empty", got.StatusLine.Dir)
	}
}
