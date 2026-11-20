package main

import (
	"bytes"
	"reflect"
	"testing"
)

func TestDialogueEntriesShowsOnlyDialogue(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []dialogueEntry
	}{
		{
			name: "typed prompt shown",
			line: `{"type":"user","message":{"role":"user","content":"rename the widget"}}`,
			want: []dialogueEntry{{kind: promptEntry, text: "rename the widget"}},
		},
		{
			name: "isMeta user record hidden",
			line: `{"type":"user","isMeta":true,"message":{"role":"user","content":"Continue from where you left off."}}`,
		},
		{
			name: "pasted_content prompt shown as the text it holds",
			line: `{"type":"user","message":{"role":"user","content":` +
				`"<pasted_content id=\"a1b2\">\nbuild log line\n</pasted_content id=\"a1b2\">\n\nwhy did it fail"}}`,
			want: []dialogueEntry{{kind: promptEntry, text: "build log line\n\nwhy did it fail"}},
		},
		{
			name: "user tool_result hidden",
			line: `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`,
		},
		{
			name: "assistant text blocks shown as is, thinking and tool_use hidden",
			line: `{"type":"assistant","message":{"role":"assistant","content":[` +
				`{"type":"thinking","thinking":"weigh options"},` +
				`{"type":"text","text":"Renaming it **now**."},` +
				`{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"ls"}},` +
				`{"type":"text","text":"Done."}]}}`,
			want: []dialogueEntry{{kind: replyEntry, text: "Renaming it **now**."}, {kind: replyEntry, text: "Done."}},
		},
		{
			name: "assistant with only tool_use hidden",
			line: `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t3","name":"Edit","input":{}}]}}`,
		},
		{
			name: "AskUserQuestion shown as each question and its option labels",
			line: `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t4",` +
				`"name":"AskUserQuestion","input":{"questions":[` +
				`{"question":"Which color?","header":"Color","multiSelect":false,"options":[` +
				`{"label":"Red (Recommended)","description":"warm"},{"label":"Blue","description":"cool"}]},` +
				`{"question":"Which size?","header":"Size","multiSelect":false,"options":[{"label":"Small","description":"s"}]}]}}]}}`,
			want: []dialogueEntry{
				{kind: questionEntry, text: "Which color?", options: []string{"Red (Recommended)", "Blue"}},
				{kind: questionEntry, text: "Which size?", options: []string{"Small"}},
			},
		},
		{
			name: "attachment record hidden even when it holds a text block",
			line: `{"type":"attachment","attachment":{"type":"text","text":"file body"}}`,
		},
		{
			name: "system record hidden",
			line: `{"type":"system","subtype":"info","content":"compacted"}`,
		},
		{
			name: "system away_summary recap hidden",
			line: `{"type":"system","subtype":"away_summary","content":"We're renaming the widget. Next, the tests."}`,
		},
		{
			name: "file-history-snapshot record hidden",
			line: `{"type":"file-history-snapshot","messageId":"m1","snapshot":{}}`,
		},
		{
			name: "api-request record hidden even when it holds an assistant text block",
			line: `{"type":"api-request","body":{"messages":[{"role":"assistant","content":[{"type":"text","text":"old"}]}]},` +
				`"echo":{"type":"assistant"}}`,
		},
	}
	for _, prefix := range notPromptPrefixes {
		cases = append(cases, struct {
			name string
			line string
			want []dialogueEntry
		}{
			name: "user record starting with " + prefix + " hidden",
			line: `{"type":"user","message":{"role":"user","content":"` + prefix + `>body</x>"}}`,
		})
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			entries, err := DialogueEntries([]byte(testCase.line))
			if err != nil {
				t.Fatalf("DialogueEntries: %v", err)
			}
			if !reflect.DeepEqual(entries, testCase.want) {
				t.Errorf("entries = %+v, want %+v", entries, testCase.want)
			}
		})
	}
}

func TestTailStartSkipsCutFirstLine(t *testing.T) {
	content := []byte("first line\nsecond line\nthird\n")
	tailBytes := int64(len("cond line\nthird\n"))
	start, err := TailStart(bytes.NewReader(content), int64(len(content)), tailBytes)
	if err != nil {
		t.Fatalf("TailStart: %v", err)
	}
	if got := string(content[start:]); got != "third\n" {
		t.Errorf("tail = %q, want %q", got, "third\n")
	}
}

func TestTailStartKeepsFirstLineCutAtItsStart(t *testing.T) {
	content := []byte("first line\nsecond line\nthird\n")
	tailBytes := int64(len("second line\nthird\n"))
	start, err := TailStart(bytes.NewReader(content), int64(len(content)), tailBytes)
	if err != nil {
		t.Fatalf("TailStart: %v", err)
	}
	if got := string(content[start:]); got != "second line\nthird\n" {
		t.Errorf("tail = %q, want %q", got, "second line\nthird\n")
	}
}

func TestTailStartReadsFileUnderTailBytesWhole(t *testing.T) {
	content := []byte("first line\nsecond line\n")
	start, err := TailStart(bytes.NewReader(content), int64(len(content)), 2*1024*1024)
	if err != nil {
		t.Fatalf("TailStart: %v", err)
	}
	if start != 0 {
		t.Errorf("start = %d, want 0", start)
	}
}

func TestTailStartFindsNewlinePastFirstChunk(t *testing.T) {
	content := append([]byte("first\n"), bytes.Repeat([]byte("x"), 70*1024)...)
	content = append(content, []byte("\nlast\n")...)
	tailBytes := int64(len(content) - len("first\n") - 1)
	start, err := TailStart(bytes.NewReader(content), int64(len(content)), tailBytes)
	if err != nil {
		t.Fatalf("TailStart: %v", err)
	}
	if got := string(content[start:]); got != "last\n" {
		t.Errorf("tail = %q, want %q", got, "last\n")
	}
}

func TestTailStartWithoutNewlineAfterCutIsFileSize(t *testing.T) {
	content := []byte("first line\nsecond line without end")
	tailBytes := int64(len("line without end"))
	start, err := TailStart(bytes.NewReader(content), int64(len(content)), tailBytes)
	if err != nil {
		t.Fatalf("TailStart: %v", err)
	}
	if start != int64(len(content)) {
		t.Errorf("start = %d, want %d", start, len(content))
	}
}
