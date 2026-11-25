package claudecode

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDialogueEntries(t *testing.T) {
	t.Run("dialogueEntriesShowTypedPrompt", dialogueEntriesShowTypedPrompt)
	t.Run("dialogueEntriesHideMetaUserRecord", dialogueEntriesHideMetaUserRecord)
	t.Run("dialogueEntriesShowPastedContentPromptAsTextItHolds", dialogueEntriesShowPastedContentPromptAsTextItHolds)
	t.Run("dialogueEntriesKeepPastedContentOverTwoLineBreaksAsPaste", dialogueEntriesKeepPastedContentOverTwoLineBreaksAsPaste)
	t.Run("dialogueEntriesShowCompactSummaryAsOneEntryWithoutItsText", dialogueEntriesShowCompactSummaryAsOneEntryWithoutItsText)
	t.Run("dialogueEntriesHideUserToolResult", dialogueEntriesHideUserToolResult)
	t.Run("dialogueEntriesShowAssistantTextBlocksAndHideThinkingAndToolUse",
		dialogueEntriesShowAssistantTextBlocksAndHideThinkingAndToolUse)
	t.Run("dialogueEntriesHideAssistantRecordOfOnlyToolUse", dialogueEntriesHideAssistantRecordOfOnlyToolUse)
	t.Run("dialogueEntriesShowAskUserQuestionAsEachQuestionAndItsOptionLabels",
		dialogueEntriesShowAskUserQuestionAsEachQuestionAndItsOptionLabels)
	t.Run("dialogueEntriesHideAttachmentRecordHoldingTextBlock", dialogueEntriesHideAttachmentRecordHoldingTextBlock)
	t.Run("dialogueEntriesShowPromptQueuedWhileClaudeWorks", dialogueEntriesShowPromptQueuedWhileClaudeWorks)
	t.Run("dialogueEntriesShowQueuedPastedContentPromptAsTextItHolds", dialogueEntriesShowQueuedPastedContentPromptAsTextItHolds)
	t.Run("dialogueEntriesHideQueuedAgentMessage", dialogueEntriesHideQueuedAgentMessage)
	t.Run("dialogueEntriesHideQueuedTaskNotification", dialogueEntriesHideQueuedTaskNotification)
	t.Run("dialogueEntriesHideQueuedTaskNotificationWithPlainTextPrompt", dialogueEntriesHideQueuedTaskNotificationWithPlainTextPrompt)
	t.Run("dialogueEntriesHideSystemRecord", dialogueEntriesHideSystemRecord)
	t.Run("dialogueEntriesHideAwaySummaryRecap", dialogueEntriesHideAwaySummaryRecap)
	t.Run("dialogueEntriesShowSlashCommandAsItsNameAndArguments", dialogueEntriesShowSlashCommandAsItsNameAndArguments)
	t.Run("dialogueEntriesShowSlashCommandWithEmptyArgumentsAsItsName", dialogueEntriesShowSlashCommandWithEmptyArgumentsAsItsName)
	t.Run("dialogueEntriesHideClearCommand", dialogueEntriesHideClearCommand)
	t.Run("dialogueEntriesShowLocalCommandRecordAsCommandItHolds", dialogueEntriesShowLocalCommandRecordAsCommandItHolds)
	t.Run("dialogueEntriesShowLocalCommandOutputWithoutColorCodes", dialogueEntriesShowLocalCommandOutputWithoutColorCodes)
	t.Run("dialogueEntriesHideEmptyLocalCommandOutput", dialogueEntriesHideEmptyLocalCommandOutput)
	t.Run("dialogueEntriesHideFileHistorySnapshotRecord", dialogueEntriesHideFileHistorySnapshotRecord)
	t.Run("dialogueEntriesHideApiRequestRecordHoldingAssistantTextBlock", dialogueEntriesHideApiRequestRecordHoldingAssistantTextBlock)
	t.Run("dialogueEntriesHideUserRecordStartingWithNotPromptPrefix", dialogueEntriesHideUserRecordStartingWithNotPromptPrefix)
}

func dialogueEntriesOfLine(t *testing.T, line string) []DialogueEntry {
	t.Helper()
	entries, err := DialogueEntries([]byte(line))
	require.NoError(t, err)
	return entries
}

func dialogueEntriesShowTypedPrompt(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"user","message":{"role":"user","content":"rename the widget"}}`)
	want := []DialogueEntry{{Kind: PromptEntry, Text: "rename the widget"}}
	require.Equal(t, want, entries)
}

func dialogueEntriesHideMetaUserRecord(t *testing.T) {
	entries := dialogueEntriesOfLine(t,
		`{"type":"user","isMeta":true,"message":{"role":"user","content":"Continue from where you left off."}}`)
	require.Nil(t, entries)
}

func dialogueEntriesShowPastedContentPromptAsTextItHolds(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"user","message":{"role":"user","content":`+
		`"<pasted_content id=\"a1b2\">\nbuild log line\n</pasted_content id=\"a1b2\">\n\nwhy did it fail"}}`)
	want := []DialogueEntry{{Kind: PromptEntry, Text: "build log line\n\nwhy did it fail"}}
	require.Equal(t, want, entries)
}

func dialogueEntriesKeepPastedContentOverTwoLineBreaksAsPaste(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"user","message":{"role":"user","content":`+
		`"<pasted_content id=\"a1b2\">\nl1\nl2\nl3\nl4\n</pasted_content id=\"a1b2\">\n\nwhy did it fail"}}`)
	want := []DialogueEntry{{Kind: PromptEntry, Text: "l1\nl2\nl3\nl4\n\nwhy did it fail", Pastes: []string{"l1\nl2\nl3\nl4"}}}
	require.Equal(t, want, entries)
}

func dialogueEntriesShowCompactSummaryAsOneEntryWithoutItsText(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"user","isVisibleInTranscriptOnly":true,"isCompactSummary":true,`+
		`"message":{"role":"user","content":"This session is being continued from a previous conversation."}}`)
	want := []DialogueEntry{{Kind: CompactSummaryEntry}}
	require.Equal(t, want, entries)
}

func dialogueEntriesHideUserToolResult(t *testing.T) {
	entries := dialogueEntriesOfLine(t,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`)
	require.Nil(t, entries)
}

func dialogueEntriesShowAssistantTextBlocksAndHideThinkingAndToolUse(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"assistant","message":{"role":"assistant","content":[`+
		`{"type":"thinking","thinking":"weigh options"},`+
		`{"type":"text","text":"Renaming it **now**."},`+
		`{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"ls"}},`+
		`{"type":"text","text":"Done."}]}}`)
	want := []DialogueEntry{{Kind: ReplyEntry, Text: "Renaming it **now**."}, {Kind: ReplyEntry, Text: "Done."}}
	require.Equal(t, want, entries)
}

func dialogueEntriesHideAssistantRecordOfOnlyToolUse(t *testing.T) {
	entries := dialogueEntriesOfLine(t,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t3","name":"Edit","input":{}}]}}`)
	require.Nil(t, entries)
}

func dialogueEntriesShowAskUserQuestionAsEachQuestionAndItsOptionLabels(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t4",`+
		`"name":"AskUserQuestion","input":{"questions":[`+
		`{"question":"Which color?","header":"Color","multiSelect":false,"options":[`+
		`{"label":"Red (Recommended)","description":"warm"},{"label":"Blue","description":"cool"}]},`+
		`{"question":"Which size?","header":"Size","multiSelect":false,"options":[{"label":"Small","description":"s"}]}]}}]}}`)
	want := []DialogueEntry{
		{Kind: QuestionEntry, Text: "Which color?", Options: []string{"Red (Recommended)", "Blue"}},
		{Kind: QuestionEntry, Text: "Which size?", Options: []string{"Small"}},
	}
	require.Equal(t, want, entries)
}

func dialogueEntriesHideAttachmentRecordHoldingTextBlock(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"attachment","attachment":{"type":"text","text":"file body"}}`)
	require.Nil(t, entries)
}

func dialogueEntriesShowPromptQueuedWhileClaudeWorks(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"attachment","attachment":{"type":"queued_command","prompt":"also check the logs",`+
		`"commandMode":"prompt","origin":{"kind":"human"}}}`)
	want := []DialogueEntry{{Kind: PromptEntry, Text: "also check the logs"}}
	require.Equal(t, want, entries)
}

func dialogueEntriesShowQueuedPastedContentPromptAsTextItHolds(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"attachment","attachment":{"type":"queued_command",`+
		`"prompt":"<pasted_content id=\"a1b2\">\nbuild log line\n</pasted_content id=\"a1b2\">\n\nwhy","commandMode":"prompt"}}`)
	want := []DialogueEntry{{Kind: PromptEntry, Text: "build log line\n\nwhy"}}
	require.Equal(t, want, entries)
}

func dialogueEntriesHideQueuedAgentMessage(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"attachment","attachment":{"type":"queued_command",`+
		`"prompt":"<agent-message from=\"a1\">done","commandMode":"prompt"}}`)
	require.Nil(t, entries)
}

func dialogueEntriesHideQueuedTaskNotification(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"attachment","attachment":{"type":"queued_command",`+
		`"prompt":"<task-notification>done","commandMode":"task-notification"}}`)
	require.Nil(t, entries)
}

func dialogueEntriesHideQueuedTaskNotificationWithPlainTextPrompt(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"attachment","attachment":{"type":"queued_command",`+
		`"prompt":"agent finished","commandMode":"task-notification"}}`)
	require.Nil(t, entries)
}

func dialogueEntriesHideSystemRecord(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"system","subtype":"info","content":"compacted"}`)
	require.Nil(t, entries)
}

func dialogueEntriesHideAwaySummaryRecap(t *testing.T) {
	entries := dialogueEntriesOfLine(t,
		`{"type":"system","subtype":"away_summary","content":"We're renaming the widget. Next, the tests."}`)
	require.Nil(t, entries)
}

func dialogueEntriesShowSlashCommandAsItsNameAndArguments(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"user","message":{"role":"user","content":"<command-name>/effort</command-name>\n`+
		`<command-message>effort</command-message>\n<command-args>high</command-args>"}}`)
	want := []DialogueEntry{{Kind: CommandEntry, Text: "/effort high"}}
	require.Equal(t, want, entries)
}

func dialogueEntriesShowSlashCommandWithEmptyArgumentsAsItsName(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"user","message":{"role":"user","content":"<command-name>/usage</command-name>\n`+
		`<command-message>usage</command-message>\n<command-args></command-args>"}}`)
	want := []DialogueEntry{{Kind: CommandEntry, Text: "/usage"}}
	require.Equal(t, want, entries)
}

func dialogueEntriesHideClearCommand(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"user","message":{"role":"user","content":"<command-name>/clear</command-name>\n`+
		`<command-message>clear</command-message>\n<command-args></command-args>"}}`)
	require.Nil(t, entries)
}

func dialogueEntriesShowLocalCommandRecordAsCommandItHolds(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"system","subtype":"local_command","content":"<command-name>/model</command-name>\n`+
		`<command-message>model</command-message>\n<command-args></command-args>"}`)
	want := []DialogueEntry{{Kind: CommandEntry, Text: "/model"}}
	require.Equal(t, want, entries)
}

func dialogueEntriesShowLocalCommandOutputWithoutColorCodes(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"system","subtype":"local_command","content":"<local-command-stdout>Set effort level to `+
		`\u001b[1mhigh\u001b[22m</local-command-stdout>"}`)
	want := []DialogueEntry{{Kind: CommandOutputEntry, Text: "Set effort level to high"}}
	require.Equal(t, want, entries)
}

func dialogueEntriesHideEmptyLocalCommandOutput(t *testing.T) {
	entries := dialogueEntriesOfLine(t,
		`{"type":"user","message":{"role":"user","content":"<local-command-stdout></local-command-stdout>"}}`)
	require.Nil(t, entries)
}

func dialogueEntriesHideFileHistorySnapshotRecord(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"file-history-snapshot","messageId":"m1","snapshot":{}}`)
	require.Nil(t, entries)
}

func dialogueEntriesHideApiRequestRecordHoldingAssistantTextBlock(t *testing.T) {
	entries := dialogueEntriesOfLine(t, `{"type":"api-request","body":{"messages":[{"role":"assistant",`+
		`"content":[{"type":"text","text":"old"}]}]},"echo":{"type":"assistant"}}`)
	require.Nil(t, entries)
}

func dialogueEntriesHideUserRecordStartingWithNotPromptPrefix(t *testing.T) {
	for _, prefix := range notPromptPrefixes {
		entries := dialogueEntriesOfLine(t, `{"type":"user","message":{"role":"user","content":"`+prefix+`>body</x>"}}`)
		require.Nil(t, entries, prefix)
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
	contentReader := bytes.NewReader(content)
	contentSize := int64(len(content))
	tailBytes := int64(len("cond line\nthird\n"))

	start, err := TailStart(contentReader, contentSize, tailBytes)
	require.NoError(t, err)
	tail := string(content[start:])
	require.Equal(t, "third\n", tail)
}

func tailStartKeepsFirstLineCutAtItsStart(t *testing.T) {
	content := []byte("first line\nsecond line\nthird\n")
	contentReader := bytes.NewReader(content)
	contentSize := int64(len(content))
	tailBytes := int64(len("second line\nthird\n"))

	start, err := TailStart(contentReader, contentSize, tailBytes)
	require.NoError(t, err)
	tail := string(content[start:])
	require.Equal(t, "second line\nthird\n", tail)
}

func tailStartReadsFileUnderTailBytesWhole(t *testing.T) {
	content := []byte("first line\nsecond line\n")
	contentReader := bytes.NewReader(content)
	contentSize := int64(len(content))

	start, err := TailStart(contentReader, contentSize, 2*1024*1024)
	require.NoError(t, err)
	require.Zero(t, start)
}

func tailStartFindsNewlinePastFirstChunk(t *testing.T) {
	longLine := bytes.Repeat([]byte("x"), 70*1024)
	content := append([]byte("first\n"), longLine...)
	content = append(content, []byte("\nlast\n")...)
	contentReader := bytes.NewReader(content)
	contentSize := int64(len(content))
	tailBytes := int64(len(content) - len("first\n") - 1)

	start, err := TailStart(contentReader, contentSize, tailBytes)
	require.NoError(t, err)
	tail := string(content[start:])
	require.Equal(t, "last\n", tail)
}

func tailStartIsFileSizeWithoutNewlineAfterCut(t *testing.T) {
	content := []byte("first line\nsecond line without end")
	contentReader := bytes.NewReader(content)
	contentSize := int64(len(content))
	tailBytes := int64(len("line without end"))

	start, err := TailStart(contentReader, contentSize, tailBytes)
	require.NoError(t, err)
	require.Equal(t, contentSize, start)
}

func TestQueueOperations(t *testing.T) {
	t.Run("queueOperationsReadEnqueueWithPastedContentAsTypedText", queueOperationsReadEnqueueWithPastedContentAsTypedText)
	t.Run("queueOperationsReadDequeueWithoutText", queueOperationsReadDequeueWithoutText)
	t.Run("queueOperationsReadRemoveWithItsReason", queueOperationsReadRemoveWithItsReason)
	t.Run("queueOperationsAreNoneForUserRecordQuotingQueueOperationType", queueOperationsAreNoneForUserRecordQuotingQueueOperationType)
	t.Run("queueOperationsAreNoneForOtherRecord", queueOperationsAreNoneForOtherRecord)
}

func queueOperationsOfLine(t *testing.T, line string) []QueueOperation {
	t.Helper()
	operations, err := QueueOperations([]byte(line))
	require.NoError(t, err)
	return operations
}

func queueOperationsReadEnqueueWithPastedContentAsTypedText(t *testing.T) {
	operations := queueOperationsOfLine(t, `{"type":"queue-operation","operation":"enqueue","content":`+
		`"<pasted_content id=\"a1\">\nlog line\n</pasted_content id=\"a1\">\n\nwhy"}`)
	want := []QueueOperation{{Operation: "enqueue", Text: "log line\n\nwhy"}}
	require.Equal(t, want, operations)
}

func queueOperationsReadDequeueWithoutText(t *testing.T) {
	operations := queueOperationsOfLine(t, `{"type":"queue-operation","operation":"dequeue"}`)
	want := []QueueOperation{{Operation: "dequeue"}}
	require.Equal(t, want, operations)
}

func queueOperationsReadRemoveWithItsReason(t *testing.T) {
	operations := queueOperationsOfLine(t,
		`{"type":"queue-operation","operation":"remove","reason":"absorbed_mid_turn","content":"go on"}`)
	want := []QueueOperation{{Operation: "remove", Reason: "absorbed_mid_turn", Text: "go on"}}
	require.Equal(t, want, operations)
}

func queueOperationsAreNoneForUserRecordQuotingQueueOperationType(t *testing.T) {
	operations := queueOperationsOfLine(t,
		`{"type":"user","message":{"role":"user","content":"what is \"type\":\"queue-operation\"?"}}`)
	require.Nil(t, operations)
}

func queueOperationsAreNoneForOtherRecord(t *testing.T) {
	operations := queueOperationsOfLine(t,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]}}`)
	require.Nil(t, operations)
}
