package ptyagent

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/stretchr/testify/require"
)

func TestOSC(t *testing.T) {
	t.Run("nonASCIIInsideOSCIsDroppedAcrossReads", nonASCIIInsideOSCIsDroppedAcrossReads)
	t.Run("utf8OutsideOSCAndASCIIQueriesPassWhole", utf8OutsideOSCAndASCIIQueriesPassWhole)
	t.Run("cancelEndsOSCSoLaterUTF8PassesWhole", cancelEndsOSCSoLaterUTF8PassesWhole)
	t.Run("substituteEndsOSCSoLaterUTF8PassesWhole", substituteEndsOSCSoLaterUTF8PassesWhole)
	t.Run("titleWithStarGlyphLeavesInputRowBlank", titleWithStarGlyphLeavesInputRowBlank)
}

func assertTerminatorEndsOSCSoLaterUTF8PassesWhole(t *testing.T, terminator string) {
	output, state := withoutNonASCIIInOSC([]byte("\x1b]0;✳"+terminator+"❯ b"), outsideOSC)
	want := "\x1b]0;" + terminator + "❯ b"
	require.Equal(t, want, string(output))
	require.Equal(t, outsideOSC, state)
}

func cancelEndsOSCSoLaterUTF8PassesWhole(t *testing.T) {
	assertTerminatorEndsOSCSoLaterUTF8PassesWhole(t, "\x18")
}

func substituteEndsOSCSoLaterUTF8PassesWhole(t *testing.T) {
	assertTerminatorEndsOSCSoLaterUTF8PassesWhole(t, "\x1a")
}

func nonASCIIInsideOSCIsDroppedAcrossReads(t *testing.T) {
	firstRead, state := withoutNonASCIIInOSC([]byte("❯ a\x1b]0;\xe2\x9c"), outsideOSC)
	secondRead, state := withoutNonASCIIInOSC([]byte("\xb3 Poem\a❯ b"), state)
	output := string(firstRead) + string(secondRead)
	require.Equal(t, "❯ a\x1b]0; Poem\a❯ b", output)
	require.Equal(t, outsideOSC, state)
}

func utf8OutsideOSCAndASCIIQueriesPassWhole(t *testing.T) {
	output := "\x1b[32;1H❯ write\x1b]11;?\x1b\\─✳\x1b]7501;?\a"
	got, _ := withoutNonASCIIInOSC([]byte(output), outsideOSC)
	require.Equal(t, output, string(got))
}

// titleWithStarGlyphLeavesInputRowBlank replays the bytes Claude Code 2.1.330 wrote after an interrupt: its title,
// then the prompt it put back, with each space drawn as a cursor move over a cell it took to be blank.
func titleWithStarGlyphLeavesInputRowBlank(t *testing.T) {
	emulator := vt.NewEmulator(40, 3)
	output := "\x1b[2;1H❯ \x1b]0;✳ Poem about rivers\a\x1b[2;1H❯ write\x1b[1Ca\x1b[1C40"
	parsableOutput, _ := withoutNonASCIIInOSC([]byte(output), outsideOSC)
	_, err := emulator.Write(parsableOutput)
	require.NoError(t, err)
	rows := strings.Split(emulator.String(), "\n")
	inputRow := strings.TrimRight(rows[1], " ")
	require.Equal(t, "❯ write a 40", inputRow)
}
