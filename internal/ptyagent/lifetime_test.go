package ptyagent

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// fakeZenEnv makes the test binary stand in for zen: it starts the child fakeZenChildEnv names, prints the
// child's PID once the child is ready, and exits once its stdin closes. With zenInJob it first puts itself in its
// kill-on-close job.
const fakeZenEnv = "ZEN_FAKE_ZEN"

// fakeZenChildEnv makes the test binary stand in for Claude, printing a line once ready: childSleeps sleeps,
// childWaitsForParent exits once parentExit reports its parent exited.
const fakeZenChildEnv = "ZEN_FAKE_ZEN_CHILD"

const (
	zenInJob    = "in-job"
	zenOutOfJob = "out-of-job"
)

const (
	childSleeps         = "sleeps"
	childWaitsForParent = "waits-for-parent"
)

func runFakeZen(zenMode string, childMode string) error {
	if zenMode == zenInJob {
		if err := putZenInKillOnCloseJob(); err != nil {
			return err
		}
	}
	child := exec.Command(os.Args[0])
	child.Env = append(os.Environ(), fakeZenEnv+"=", fakeZenChildEnv+"="+childMode)
	childStdout, err := child.StdoutPipe()
	if err != nil {
		return err
	}
	if err := child.Start(); err != nil {
		return err
	}
	if _, err := bufio.NewReader(childStdout).ReadString('\n'); err != nil {
		return fmt.Errorf("wait for the child to be ready: %w", err)
	}
	fmt.Println(child.Process.Pid)
	_, err = io.Copy(io.Discard, os.Stdin)
	return err
}

func runFakeZenChild(childMode string) error {
	if childMode == childSleeps {
		fmt.Println("ready")
		time.Sleep(time.Hour)
		return nil
	}
	parentExited, err := parentExit()
	if err != nil {
		return err
	}
	fmt.Println("ready")
	return <-parentExited
}

func TestLifetime(t *testing.T) {
	t.Run("childInZensJobEndsWhenZenExits", childInZensJobEndsWhenZenExits)
	t.Run("parentExitReportsWhenParentExits", parentExitReportsWhenParentExits)
}

func childInZensJobEndsWhenZenExits(t *testing.T) {
	childExitCodeOnceZenEnds(t, zenInJob, childSleeps)
}

func parentExitReportsWhenParentExits(t *testing.T) {
	exitCode := childExitCodeOnceZenEnds(t, zenOutOfJob, childWaitsForParent)
	require.Zero(t, exitCode, "parentExit reports zen's exit with no error")
}

// childExitCodeOnceZenEnds fails the test when the child of a fake zen still runs 10s after the fake zen exits.
func childExitCodeOnceZenEnds(t *testing.T, zenMode string, childMode string) uint32 {
	zen := exec.Command(os.Args[0])
	zen.Stderr = os.Stderr
	zen.Env = append(os.Environ(), fakeZenEnv+"="+zenMode, fakeZenChildEnv+"="+childMode)
	zenStdin, err := zen.StdinPipe()
	require.NoError(t, err)
	zenStdout, err := zen.StdoutPipe()
	require.NoError(t, err)
	startErr := zen.Start()
	require.NoError(t, startErr)
	childPIDLine, err := bufio.NewReader(zenStdout).ReadString('\n')
	require.NoError(t, err)
	childPIDText := strings.TrimSpace(childPIDLine)
	childPID, err := strconv.ParseUint(childPIDText, 10, 32)
	require.NoError(t, err)
	child, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false,
		uint32(childPID))
	require.NoError(t, err)
	t.Cleanup(func() {
		closeErr := windows.CloseHandle(child)
		require.NoError(t, closeErr)
	})
	closeStdinErr := zenStdin.Close()
	require.NoError(t, closeStdinErr)
	zenErr := zen.Wait()
	require.NoError(t, zenErr)
	waitResult, err := windows.WaitForSingleObject(child, 10_000)
	require.NoError(t, err)
	childExited := waitResult == windows.WAIT_OBJECT_0
	if !childExited {
		terminateErr := windows.TerminateProcess(child, 1)
		require.NoError(t, terminateErr)
	}
	require.True(t, childExited, "child %d still runs 10s after its parent exited", childPID)
	var exitCode uint32
	exitCodeErr := windows.GetExitCodeProcess(child, &exitCode)
	require.NoError(t, exitCodeErr)
	return exitCode
}
