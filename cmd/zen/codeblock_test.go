package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestReplyRowsDrawEachCodeLineOnASurfaceBoxTheReplyWidth(t *testing.T) {
	reply := "Run this:\n\n```powershell\nGet-Process zen |\n\tStop-Process -Force\n```\n\nThen check it is gone."
	for _, width := range []int{40, 100} {
		replyRenderer, err := NewReplyRenderer(width)
		if err != nil {
			t.Fatalf("NewReplyRenderer(%d): %v", width, err)
		}
		rows, err := ReplyRows(reply, width, replyRenderer)
		if err != nil {
			t.Fatalf("ReplyRows at width %d: %v", width, err)
		}
		boxRows := []string{}
		for _, row := range rows {
			if strings.HasPrefix(row, surfaceBackground) {
				boxRows = append(boxRows, row)
			}
		}
		plainBox := []string{}
		for _, row := range boxRows {
			plainBox = append(plainBox, strings.TrimRight(ansi.Strip(row), " "))
			if rowWidth := ansi.StringWidth(row); rowWidth != width-2 {
				t.Errorf("width %d: box row %q is %d columns wide, want %d", width, ansi.Strip(row), rowWidth, width-2)
			}
			// every reset inside the row is followed by the surface again, so no cell loses the background
			inside := strings.TrimSuffix(row, ansi.ResetStyle)
			if strings.Count(inside, "\x1b[0m") != strings.Count(inside, "\x1b[0m"+surfaceBackground) {
				t.Errorf("width %d: box row %q drops the surface after a reset", width, row)
			}
		}
		want := []string{"", "  Get-Process zen |", "      Stop-Process -Force", ""}
		if !reflect.DeepEqual(plainBox, want) {
			t.Errorf("width %d: box rows = %q, want %q", width, plainBox, want)
		}
		plain := ansi.Strip(strings.Join(rows, "\n"))
		if !strings.Contains(plain, "Run this:") || !strings.Contains(plain, "Then check it is gone.") ||
			strings.Contains(plain, "ZENCODEBLOCK") {
			t.Errorf("width %d: reply rows = %q, want the prose around the box and no placeholder", width, plain)
		}
	}
}

func TestCodeBoxRowsWrapALongLineOnTheSurfaceAtTheBoxWidth(t *testing.T) {
	block := codeBlock{code: "Get-Process -Name zen | Stop-Process -Force -PassThru -Verbose", language: "powershell"}
	rows, err := CodeBoxRows(block, 30)
	if err != nil {
		t.Fatalf("CodeBoxRows: %v", err)
	}
	if len(rows) < 4 {
		t.Fatalf("rows = %q, want the code wrapped onto at least two rows between the blank rows", rows)
	}
	codeText := ""
	for _, row := range rows {
		if !strings.HasPrefix(row, surfaceBackground) {
			t.Errorf("row %q does not start on the surface", row)
		}
		if rowWidth := ansi.StringWidth(row); rowWidth != 30 {
			t.Errorf("row %q is %d columns wide, want 30", ansi.Strip(row), rowWidth)
		}
		inside := strings.TrimSuffix(row, ansi.ResetStyle)
		if strings.Count(inside, "\x1b[0m") != strings.Count(inside, "\x1b[0m"+surfaceBackground) {
			t.Errorf("row %q drops the surface after a reset", row)
		}
		codeText += ansi.Strip(row)
	}
	if strings.ReplaceAll(codeText, " ", "") != strings.ReplaceAll(block.code, " ", "") {
		t.Errorf("wrapped rows hold %q, want the characters of %q", codeText, block.code)
	}
}

func TestReplyRowsKeepTheTextAroundACodeBlock(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		want  []string
	}{
		{name: "in a list item", reply: "1. First step:\n   ```sh\n   echo one\n   ```\n   then this.\n2. Second step",
			want: []string{"", "1. First step:", "", "echo one", "", "then this.", "2. Second step"}},
		{name: "in a quote", reply: "> Run:\n> ```sh\n> echo one\n> ```\n> done",
			want: []string{"", "│ Run:", "│", "", "echo one", "", "│", "│ done"}},
		{name: "prose that spells the placeholder", reply: "The row ZENCODEBLOCK0 stays text.\n\n```\necho one\n```",
			want: []string{"The row ZENCODEBLOCK0 stays text.", "", "", "echo one", ""}},
	}
	replyRenderer, err := NewReplyRenderer(60)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			rows, err := ReplyRows(testCase.reply, 60, replyRenderer)
			if err != nil {
				t.Fatalf("ReplyRows: %v", err)
			}
			plain := []string{}
			for _, row := range rows {
				plain = append(plain, strings.TrimSpace(ansi.Strip(row)))
			}
			if !reflect.DeepEqual(plain, testCase.want) {
				t.Errorf("rows = %q, want %q", plain, testCase.want)
			}
		})
	}
}

func TestCodeBlocksHoldTheCodeExactlyAsInTheReply(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		want  []string
	}{
		{name: "a tab and a trailing space", reply: "Run:\n\n```go\nfunc f() {\n\treturn \n}\n```\n",
			want: []string{"func f() {\n\treturn \n}"}},
		{name: "in a list item", reply: "1. Step:\n   ```sh\n   echo one\n     indented\n   ```\n2. Next",
			want: []string{"echo one\n  indented"}},
		{name: "in a quote", reply: "> ```\n> inside\n> ```", want: []string{"inside"}},
		{name: "tilde fence left open", reply: "~~~\nfirst\n\nlast", want: []string{"first\n\nlast"}},
		{name: "two blocks", reply: "```\none\n```\n\n```\ntwo\n```", want: []string{"one", "two"}},
		{name: "empty block", reply: "```\n```", want: []string{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			blocks, err := CodeBlocks(testCase.reply)
			if err != nil {
				t.Fatalf("CodeBlocks: %v", err)
			}
			codes := []string{}
			for _, block := range blocks {
				codes = append(codes, block.code)
			}
			if !reflect.DeepEqual(codes, testCase.want) {
				t.Errorf("codes = %q, want %q", codes, testCase.want)
			}
		})
	}
}
