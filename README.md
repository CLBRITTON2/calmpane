# calmpane

A quiet view for Claude Code: prompts, replies and questions, without tool calls, tool results or thinking, and you
type and send prompts from it. It runs in any terminal (`calmpane claude`) or as a
[herdr](https://github.com/herdrdev/herdr) plugin, zoomed over the Claude pane. The public `claudecode` Go package
reads Claude Code transcripts for other tools.

> [!NOTE]
> Zen only hides the clutter to make Claude's replies easier to read. You should always be aware of what commands an
> LLM is running and what files it is editing, by whatever means fit your workflow.

## Install

```
go install github.com/CLBRITTON2/calmpane/cmd/calmpane@latest
```

From a clone, `make` installs it and `make test` vets and tests it.

To run calmpane in any terminal without herdr, start Claude through it, with any arguments Claude takes:

```
calmpane claude
calmpane claude --continue
```

alt+x switches between the zen view and Claude's own screen. calmpane exits with Claude's exit code when Claude exits.

`calmpane.exe` also runs as a herdr plugin. The plugin manifest and key bindings live in
[windows-dev](https://github.com/CLBRITTON2/windows-dev) (`herdr/zen`, `herdr/config.toml`).

## Keys

| Key          | Action                                                            |
|--------------|-------------------------------------------------------------------|
| enter        | Send the prompt                                                   |
| shift+enter  | New line                                                          |
| ctrl+enter   | Send the prompt now, even while Claude works                      |
| esc          | Interrupt Claude while it works                                   |
| ctrl+y       | Copy the newest code block                                        |
| up, down     | Scroll three rows, or move the cursor in an input of several rows |
| pgup, pgdown | Scroll a page                                                     |
| end          | Jump to the newest entry                                          |

## Reading Claude Code transcripts from Go

The `claudecode` package reads a Claude Code session for other tools: its transcript, the prompts, replies and
questions its records hold, and Claude's input box on a captured screen. The transcript format is Claude Code's own,
not a stable API, so a Claude Code release can change what it reads.

```go
import "github.com/CLBRITTON2/calmpane/claudecode"

// sessionID is the session_id Claude Code passes its hooks. WaitForTranscript sends each session whose transcript
// holds the conversation as it finds it, which a /resume of a background session changes.
conversationSessionIDs := make(chan string)
go func() {
	for range conversationSessionIDs {
	}
}()
transcript, err := claudecode.WaitForTranscript(sessionID, conversationSessionIDs)
if err != nil {
	return err
}
defer transcript.Close()
scanner := bufio.NewScanner(transcript)
scanner.Buffer(nil, 64*1024*1024)
for scanner.Scan() {
	entries, err := claudecode.DialogueEntries(scanner.Bytes())
	if err != nil {
		return err
	}
	for _, entry := range entries {
		fmt.Println(entry.Kind, entry.Text)
	}
}
return scanner.Err()
```

`TailStart` finds where to start reading the last bytes of a long transcript.

## License

[Apache-2.0](LICENSE)
