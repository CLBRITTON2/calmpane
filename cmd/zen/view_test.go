package main

import "testing"

func TestEscapeSequenceLengthConsumesWholeSequence(t *testing.T) {
	cases := []struct {
		name string
		keys string
		want int
	}{
		{name: "CSI arrow", keys: "\x1b[Aab", want: 3},
		{name: "CSI with parameters", keys: "\x1b[1;5Cab", want: 6},
		{name: "CSI cut before its final byte", keys: "\x1b[1;5", want: 5},
		{name: "SS3 function key", keys: "\x1bOPab", want: 3},
		{name: "alt chord", keys: "\x1bzab", want: 2},
		{name: "lone ESC", keys: "\x1b", want: 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := escapeSequenceLength([]byte(testCase.keys))
			if got != testCase.want {
				t.Errorf("escapeSequenceLength(%q) = %d, want %d", testCase.keys, got, testCase.want)
			}
		})
	}
}

func TestRowsOfCountsWrappedRows(t *testing.T) {
	cases := []struct {
		name string
		line string
		want int
	}{
		{name: "empty line takes a row", line: "", want: 1},
		{name: "shorter than the width", line: "abc", want: 1},
		{name: "exactly the width", line: "abcd", want: 1},
		{name: "one rune over the width", line: "abcde", want: 2},
		{name: "multi-byte runes count once", line: "ééééé", want: 2},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := rowsOf(testCase.line, 4)
			if got != testCase.want {
				t.Errorf("rowsOf(%q, 4) = %d, want %d", testCase.line, got, testCase.want)
			}
		})
	}
}
