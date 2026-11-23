package claudecode

import (
	"bytes"
	"reflect"
	"testing"
)

func TestDialogueEntries(t *testing.T) {
	t.Run("dialogueEntriesShowOnlyDialogue", dialogueEntriesShowOnlyDialogue)
}

func dialogueEntriesShowOnlyDialogue(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []DialogueEntry
	}{
		{
			name: "typed prompt shown",
			line: `{"type":"user","message":{"role":"user","content":"rename the widget"}}`,
			want: []DialogueEntry{{Kind: PromptEntry, Text: "rename the widget"}},
		},
		{
			name: "isMeta user record hidden",
			line: `{"type":"user","isMeta":true,"message":{"role":"user","content":"Continue from where you left off."}}`,
		},
		{
			name: "pasted_content prompt shown as the text it holds",
			line: `{"type":"user","message":{"role":"user","content":` +
				`"<pasted_content id=\"a1b2\">\nbuild log line\n</pasted_content id=\"a1b2\">\n\nwhy did it fail"}}`,
			want: []DialogueEntry{{Kind: PromptEntry, Text: "build log line\n\nwhy did it fail"}},
		},
		{
			name: "pasted_content of more than two line breaks kept as a paste",
			line: `{"type":"user","message":{"role":"user","content":` +
				`"<pasted_content id=\"a1b2\">\nl1\nl2\nl3\nl4\n</pasted_content id=\"a1b2\">\n\nwhy did it fail"}}`,
			want: []DialogueEntry{{Kind: PromptEntry, Text: "l1\nl2\nl3\nl4\n\nwhy did it fail", Pastes: []string{"l1\nl2\nl3\nl4"}}},
		},
		{
			name: "compact summary shown as one entry without its text",
			line: `{"type":"user","isVisibleInTranscriptOnly":true,"isCompactSummary":true,"message":{"role":"user",` +
				`"content":"This session is being continued from a previous conversation."}}`,
			want: []DialogueEntry{{Kind: CompactSummaryEntry}},
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
			want: []DialogueEntry{{Kind: ReplyEntry, Text: "Renaming it **now**."}, {Kind: ReplyEntry, Text: "Done."}},
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
			want: []DialogueEntry{
				{Kind: QuestionEntry, Text: "Which color?", Options: []string{"Red (Recommended)", "Blue"}},
				{Kind: QuestionEntry, Text: "Which size?", Options: []string{"Small"}},
			},
		},
		{
			name: "attachment record hidden even when it holds a text block",
			line: `{"type":"attachment","attachment":{"type":"text","text":"file body"}}`,
		},
		{
			name: "prompt queued while Claude works shown",
			line: `{"type":"attachment","attachment":{"type":"queued_command","prompt":"also check the logs",` +
				`"commandMode":"prompt","origin":{"kind":"human"}}}`,
			want: []DialogueEntry{{Kind: PromptEntry, Text: "also check the logs"}},
		},
		{
			name: "queued pasted_content prompt shown as the text it holds",
			line: `{"type":"attachment","attachment":{"type":"queued_command",` +
				`"prompt":"<pasted_content id=\"a1b2\">\nbuild log line\n</pasted_content id=\"a1b2\">\n\nwhy","commandMode":"prompt"}}`,
			want: []DialogueEntry{{Kind: PromptEntry, Text: "build log line\n\nwhy"}},
		},
		{
			name: "queued agent message hidden",
			line: `{"type":"attachment","attachment":{"type":"queued_command","prompt":"<agent-message from=\"a1\">done",` +
				`"commandMode":"prompt"}}`,
		},
		{
			name: "queued task notification hidden",
			line: `{"type":"attachment","attachment":{"type":"queued_command","prompt":"<task-notification>done",` +
				`"commandMode":"task-notification"}}`,
		},
		{
			name: "queued task notification hidden even when its prompt is plain text",
			line: `{"type":"attachment","attachment":{"type":"queued_command","prompt":"agent finished",` +
				`"commandMode":"task-notification"}}`,
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
			name: "slash command shown as its name and arguments",
			line: `{"type":"user","message":{"role":"user","content":"<command-name>/effort</command-name>\n` +
				`<command-message>effort</command-message>\n<command-args>high</command-args>"}}`,
			want: []DialogueEntry{{Kind: CommandEntry, Text: "/effort high"}},
		},
		{
			name: "slash command with empty arguments shown as its name",
			line: `{"type":"user","message":{"role":"user","content":"<command-name>/usage</command-name>\n` +
				`<command-message>usage</command-message>\n<command-args></command-args>"}}`,
			want: []DialogueEntry{{Kind: CommandEntry, Text: "/usage"}},
		},
		{
			name: "clear command hidden",
			line: `{"type":"user","message":{"role":"user","content":"<command-name>/clear</command-name>\n` +
				`<command-message>clear</command-message>\n<command-args></command-args>"}}`,
		},
		{
			name: "local_command system record shown as the command it holds",
			line: `{"type":"system","subtype":"local_command","content":"<command-name>/model</command-name>\n` +
				`<command-message>model</command-message>\n<command-args></command-args>"}`,
			want: []DialogueEntry{{Kind: CommandEntry, Text: "/model"}},
		},
		{
			name: "local_command output shown without its color codes",
			line: `{"type":"system","subtype":"local_command","content":"<local-command-stdout>Set effort level to ` +
				`\u001b[1mhigh\u001b[22m</local-command-stdout>"}`,
			want: []DialogueEntry{{Kind: CommandOutputEntry, Text: "Set effort level to high"}},
		},
		{
			name: "empty local command output hidden",
			line: `{"type":"user","message":{"role":"user","content":"<local-command-stdout></local-command-stdout>"}}`,
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
			want []DialogueEntry
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

func TestTailStart(t *testing.T) {
	t.Run("tailStartSkipsCutFirstLine", tailStartSkipsCutFirstLine)
	t.Run("tailStartKeepsFirstLineCutAtItsStart", tailStartKeepsFirstLineCutAtItsStart)
	t.Run("tailStartReadsFileUnderTailBytesWhole", tailStartReadsFileUnderTailBytesWhole)
	t.Run("tailStartFindsNewlinePastFirstChunk", tailStartFindsNewlinePastFirstChunk)
	t.Run("tailStartIsFileSizeWithoutNewlineAfterCut", tailStartIsFileSizeWithoutNewlineAfterCut)
}

func tailStartSkipsCutFirstLine(t *testing.T) {
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

func tailStartKeepsFirstLineCutAtItsStart(t *testing.T) {
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

func tailStartReadsFileUnderTailBytesWhole(t *testing.T) {
	content := []byte("first line\nsecond line\n")
	start, err := TailStart(bytes.NewReader(content), int64(len(content)), 2*1024*1024)
	if err != nil {
		t.Fatalf("TailStart: %v", err)
	}
	if start != 0 {
		t.Errorf("start = %d, want 0", start)
	}
}

func tailStartFindsNewlinePastFirstChunk(t *testing.T) {
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

func tailStartIsFileSizeWithoutNewlineAfterCut(t *testing.T) {
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

func TestQueueOperations(t *testing.T) {
	t.Run("queueOperationsReadOperationReasonAndTypedText", queueOperationsReadOperationReasonAndTypedText)
}

func queueOperationsReadOperationReasonAndTypedText(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []QueueOperation
	}{
		{
			name: "enqueue with pasted content",
			line: `{"type":"queue-operation","operation":"enqueue","content":` +
				`"<pasted_content id=\"a1\">\nlog line\n</pasted_content id=\"a1\">\n\nwhy"}`,
			want: []QueueOperation{{Operation: "enqueue", Text: "log line\n\nwhy"}},
		},
		{
			name: "dequeue without text",
			line: `{"type":"queue-operation","operation":"dequeue"}`,
			want: []QueueOperation{{Operation: "dequeue"}},
		},
		{
			name: "remove with its reason",
			line: `{"type":"queue-operation","operation":"remove","reason":"absorbed_mid_turn","content":"go on"}`,
			want: []QueueOperation{{Operation: "remove", Reason: "absorbed_mid_turn", Text: "go on"}},
		},
		{
			name: "user record quoting a queue-operation type",
			line: `{"type":"user","message":{"role":"user","content":"what is \"type\":\"queue-operation\"?"}}`,
		},
		{
			name: "other record",
			line: `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]}}`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := QueueOperations([]byte(testCase.line))
			if err != nil {
				t.Fatalf("QueueOperations: %v", err)
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("QueueOperations = %+v, want %+v", got, testCase.want)
			}
		})
	}
}
