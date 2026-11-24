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

alt+x, or the key the config sets, switches between the zen view and Claude's own screen. calmpane exits
with Claude's exit code when Claude exits.

`calmpane.exe` also runs as a herdr plugin. The plugin manifest and key bindings live in
[windows-dev](https://github.com/CLBRITTON2/windows-dev) (`herdr/zen`, `herdr/config.toml`).

## Keys

| Key          | Action                                                            |
|--------------|-------------------------------------------------------------------|
| enter        | Send the prompt                                                   |
| shift+enter  | New line                                                          |
| ctrl+enter   | Send the prompt now, even while Claude works                      |
| esc          | Interrupt Claude while it works, otherwise clear the input        |
| ctrl+e       | Expand the paste label at the cursor, or collapse it back         |
| ctrl+y       | Open copy mode to copy any code block, or several                 |
| up, down     | Move the cursor, or scroll three rows from the input's edge rows  |
| pgup, pgdown | Scroll a page                                                     |
| end          | Jump to the newest entry                                          |

## Config

calmpane reads `%APPDATA%\calmpane\config.toml` when the zen view or `calmpane claude` starts, and writes the
documented [example](internal/config/config.toml) there when the file is missing. Every field is required, and a
field calmpane does not know, a missing field or a bad value stops the view with an error naming it.

- `[colors]`: the `#rrggbb` color of each part of the view.
- `[lines]`: the characters the separator and the code block frames are drawn with.
- `[code]`: the [chroma](https://github.com/alecthomas/chroma/tree/master/styles) style that highlights code blocks,
  and the style of their comments.
- `[status_line]`: the folder Claude's status line command writes to, shown under the separator.
- `[keys]`: the key that shows Claude's own screen, and the key that expands and collapses paste labels.

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
