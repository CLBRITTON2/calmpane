// Package claudecode reads Claude Code sessions: the JSONL transcript of a session under ~/.claude/projects, the
// background session a /resume opened, the prompts, replies, questions and queue operations its records hold, the
// labels Claude Code shows for long pastes, and Claude's input box on a captured screen. The transcript format is
// Claude Code's own, not a stable API: the record shapes were checked against Claude Code 2.1.329.
package claudecode
