// Package config reads calmpane's config.toml.
package config

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/pelletier/go-toml/v2"
)

// exampleDocument is written where a config is missing, so a fresh install looks as the documented example says.
//
//go:embed config.toml
var exampleDocument []byte

type Config struct {
	Colors     Colors     `toml:"colors"`
	Lines      Lines      `toml:"lines"`
	Code       Code       `toml:"code"`
	StatusLine StatusLine `toml:"status_line"`
	Keys       Keys       `toml:"keys"`
}

// Colors are #rrggbb strings.
type Colors struct {
	PromptBand   string `toml:"prompt_band"`
	PromptMarker string `toml:"prompt_marker"`
	PromptText   string `toml:"prompt_text"`
	PromptNote   string `toml:"prompt_note"`
	ReplyText    string `toml:"reply_text"`
	Heading      string `toml:"heading"`
	InlineCode   string `toml:"inline_code"`
	Link         string `toml:"link"`
	Question     string `toml:"question"`
	Option       string `toml:"option"`
	Working      string `toml:"working"`
	Waiting      string `toml:"waiting"`
	Done         string `toml:"done"`
	Error        string `toml:"error"`
	Separator    string `toml:"separator"`
	CodeRule     string `toml:"code_rule"`
	CodeLabel    string `toml:"code_label"`
}

// Lines are single characters one column wide, drawn repeated.
type Lines struct {
	Separator string `toml:"separator"`
	CodeRule  string `toml:"code_rule"`
}

type Code struct {
	// Theme is a name in chroma's style registry.
	Theme string `toml:"theme"`
	// Comment is a chroma style entry, empty to keep the theme's own comment style.
	Comment string `toml:"comment"`
}

type StatusLine struct {
	// Dir has its leading ~ expanded to the profile folder, and is empty for no status line row.
	Dir string `toml:"dir"`
}

type Keys struct {
	ClaudeScreen string `toml:"claude_screen"`
	ExpandPaste  string `toml:"expand_paste"`
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// keyCodesByName maps each name tea.Key.Keystroke writes for a key other than a printable character to its code.
var keyCodesByName = func() map[string]rune {
	codes := []rune{tea.KeyBackspace, tea.KeyTab, tea.KeyEnter, tea.KeyEscape, tea.KeySpace}
	for code := tea.KeyUp; code <= tea.KeyIsoLevel5Shift; code++ {
		codes = append(codes, code)
	}
	codesByName := map[string]rune{}
	for _, code := range codes {
		codesByName[tea.Key{Code: code}.Keystroke()] = code
	}
	return codesByName
}()

var keyModifiersByName = map[string]tea.KeyMod{"ctrl": tea.ModCtrl, "alt": tea.ModAlt, "shift": tea.ModShift,
	"meta": tea.ModMeta, "hyper": tea.ModHyper, "super": tea.ModSuper}

// Load reads the config file, first writing the example there when it is missing.
func Load(file string) (Config, error) {
	document, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return Config{}, fmt.Errorf("create the folder of %s: %w", file, err)
		}
		if err := os.WriteFile(file, exampleDocument, 0o644); err != nil {
			return Config{}, fmt.Errorf("write the example config to %s: %w", file, err)
		}
		document, err = os.ReadFile(file)
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", file, err)
	}
	config, err := Parse(document)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", file, err)
	}
	return config, nil
}

// Example is the config the example document holds.
func Example() (Config, error) {
	return Parse(exampleDocument)
}

func Parse(document []byte) (Config, error) {
	var config Config
	decodeErr := toml.NewDecoder(bytes.NewReader(document)).DisallowUnknownFields().Decode(&config)
	var unknownFields *toml.StrictMissingError
	var decodeError *toml.DecodeError
	switch {
	case errors.As(decodeErr, &unknownFields):
		problems := []error{}
		for _, unknown := range unknownFields.Errors {
			row, _ := unknown.Position()
			problems = append(problems, fmt.Errorf("unknown field %s in line %d: %s", strings.Join(unknown.Key(), "."), row,
				documentLine(document, row)))
		}
		return Config{}, errors.Join(problems...)
	case errors.As(decodeErr, &decodeError):
		row, _ := decodeError.Position()
		return Config{}, fmt.Errorf("line %d: %s: %w", row, documentLine(document, row), decodeErr)
	case decodeErr != nil:
		return Config{}, decodeErr
	}
	// strict decoding into Config let through only known sections holding string fields
	var fieldsBySection map[string]map[string]string
	if err := toml.Unmarshal(document, &fieldsBySection); err != nil {
		return Config{}, err
	}
	if missing := missingFields(fieldsBySection); len(missing) > 0 {
		return Config{}, fmt.Errorf("missing field %s", strings.Join(missing, ", "))
	}
	return checkedConfig(config)
}

func documentLine(document []byte, row int) string {
	lines := strings.Split(string(document), "\n")
	if row < 1 || row > len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[row-1])
}

// missingFields names each field of Config the document lacks, as section.field.
func missingFields(fieldsBySection map[string]map[string]string) []string {
	missing := []string{}
	sections := reflect.TypeFor[Config]()
	for sectionIndex := range sections.NumField() {
		section := sections.Field(sectionIndex)
		sectionName := section.Tag.Get("toml")
		for fieldIndex := range section.Type.NumField() {
			fieldName := section.Type.Field(fieldIndex).Tag.Get("toml")
			if _, present := fieldsBySection[sectionName][fieldName]; !present {
				missing = append(missing, sectionName+"."+fieldName)
			}
		}
	}
	return missing
}

// checkedConfig is config with the status line folder expanded, or the error naming each bad value.
func checkedConfig(config Config) (Config, error) {
	problems := []error{}
	colors := reflect.ValueOf(config.Colors)
	for index := range colors.NumField() {
		color := colors.Field(index).String()
		if !hexColor.MatchString(color) {
			problems = append(problems, fmt.Errorf("colors.%s = %q is not a #rrggbb color",
				colors.Type().Field(index).Tag.Get("toml"), color))
		}
	}
	lines := reflect.ValueOf(config.Lines)
	for index := range lines.NumField() {
		line := lines.Field(index).String()
		if utf8.RuneCountInString(line) != 1 || ansi.StringWidth(line) != 1 {
			problems = append(problems, fmt.Errorf("lines.%s = %q is not a single character one column wide",
				lines.Type().Field(index).Tag.Get("toml"), line))
		}
	}
	if _, registered := styles.Registry[strings.ToLower(config.Code.Theme)]; !registered {
		problems = append(problems, fmt.Errorf("code.theme = %q is not a chroma style", config.Code.Theme))
	}
	if config.Code.Comment != "" {
		if _, err := chroma.ParseStyleEntry(config.Code.Comment); err != nil {
			problems = append(problems, fmt.Errorf("code.comment = %q is not a chroma style entry: %w",
				config.Code.Comment, err))
		}
	}
	if config.Keys.ClaudeScreen == "" {
		problems = append(problems, errors.New(`keys.claude_screen = "" names no key`))
	}
	// zen matches a key by the string bubbletea writes for it, so the name must be one that string can be
	keyParts := strings.Split(config.Keys.ExpandPaste, "+")
	keyName := []rune(keyParts[len(keyParts)-1])
	expandPasteKey := tea.Key{Code: keyCodesByName[string(keyName)]}
	if expandPasteKey.Code == 0 && len(keyName) == 1 && unicode.IsPrint(keyName[0]) {
		expandPasteKey.Code = keyName[0]
	}
	for _, modifier := range keyParts[:len(keyParts)-1] {
		expandPasteKey.Mod |= keyModifiersByName[modifier]
	}
	if expandPasteKey.Keystroke() != config.Keys.ExpandPaste {
		problems = append(problems, fmt.Errorf("keys.expand_paste = %q is not a key name such as \"ctrl+e\"",
			config.Keys.ExpandPaste))
	}
	if rest, underProfile := strings.CutPrefix(config.StatusLine.Dir, "~"); underProfile {
		profile, err := os.UserHomeDir()
		if err != nil {
			problems = append(problems, fmt.Errorf("status_line.dir = %q: find the profile folder: %w",
				config.StatusLine.Dir, err))
		}
		config.StatusLine.Dir = filepath.Join(profile, rest)
	}
	if len(problems) > 0 {
		return Config{}, errors.Join(problems...)
	}
	return config, nil
}
