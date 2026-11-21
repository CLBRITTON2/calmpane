package ptyagent

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
)

func TestOSC(t *testing.T) {
	t.Run("nonASCIIInsideOSCIsDroppedAcrossReads", nonASCIIInsideOSCIsDroppedAcrossReads)
	t.Run("utf8OutsideOSCAndASCIIQueriesPassWhole", utf8OutsideOSCAndASCIIQueriesPassWhole)
	t.Run("cancelAndSubstituteEndOSCSoLaterUTF8PassesWhole", cancelAndSubstituteEndOSCSoLaterUTF8PassesWhole)
	t.Run("titleWithStarGlyphLeavesInputRowBlank", titleWithStarGlyphLeavesInputRowBlank)
}

func cancelAndSubstituteEndOSCSoLaterUTF8PassesWhole(t *testing.T) {
	for _, terminator := range []string{"\x18", "\x1a"} {
		got, state := withoutNonASCIIInOSC([]byte("\x1b]0;✳"+terminator+"❯ b"), outsideOSC)
		if want := "\x1b]0;" + terminator + "❯ b"; string(got) != want {
			t.Errorf("output = %q, want %q", got, want)
		}
		if state != outsideOSC {
			t.Errorf("state after %q = %d, want outsideOSC", terminator, state)
		}
	}
}

func nonASCIIInsideOSCIsDroppedAcrossReads(t *testing.T) {
	firstRead, state := withoutNonASCIIInOSC([]byte("❯ a\x1b]0;\xe2\x9c"), outsideOSC)
	secondRead, state := withoutNonASCIIInOSC([]byte("\xb3 Poem\a❯ b"), state)
	if got, want := string(firstRead)+string(secondRead), "❯ a\x1b]0; Poem\a❯ b"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if state != outsideOSC {
		t.Errorf("state after BEL = %d, want outsideOSC", state)
	}
}

func utf8OutsideOSCAndASCIIQueriesPassWhole(t *testing.T) {
	output := "\x1b[32;1H❯ write\x1b]11;?\x1b\\─✳\x1b]7501;?\a"
	if got, _ := withoutNonASCIIInOSC([]byte(output), outsideOSC); string(got) != output {
		t.Errorf("output = %q, want it unchanged", got)
	}
}

// titleWithStarGlyphLeavesInputRowBlank replays the bytes Claude Code 2.1.330 wrote after an interrupt: its title,
// then the prompt it put back, with each space drawn as a cursor move over a cell it took to be blank.
func titleWithStarGlyphLeavesInputRowBlank(t *testing.T) {
	emulator := vt.NewEmulator(40, 3)
	output := "\x1b[2;1H❯ \x1b]0;✳ Poem about rivers\a\x1b[2;1H❯ write\x1b[1Ca\x1b[1C40"
	parsableOutput, _ := withoutNonASCIIInOSC([]byte(output), outsideOSC)
	if _, err := emulator.Write(parsableOutput); err != nil {
		t.Fatal(err)
	}
	inputRow := strings.Split(emulator.String(), "\n")[1]
	if want := "❯ write a 40"; strings.TrimRight(inputRow, " ") != want {
		t.Errorf("input row = %q, want %q", inputRow, want)
	}
}
