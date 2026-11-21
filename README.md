# herdr-zen

A distraction-free view of a Claude Code pane in [herdr](https://github.com/herdrdev/herdr): prompts, replies and
questions, without tool calls, tool results or thinking. It opens zoomed over the Claude pane, and you type and send
prompts from it.

> [!NOTE]
> Zen only hides the clutter to make Claude's replies easier to read. You should always be aware of what commands an
> LLM is running and what files it is editing, by whatever means fit your workflow.

## Install

```
go install github.com/CLBRITTON2/herdr-zen/cmd/zen@latest
```

From a clone, `make` installs it and `make test` vets and tests it.

`zen.exe` runs as a herdr plugin. The plugin manifest and key bindings live in
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

## License

[Apache-2.0](LICENSE)
