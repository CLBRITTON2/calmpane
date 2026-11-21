package ptyagent

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// putZenInKillOnCloseJob puts zen in a job that Windows ends with every process in it once zen's handle to it closes,
// so Claude and its children, which join zen's job when they start, end however zen ends. The handle stays open until
// zen exits.
func putZenInKillOnCloseJob() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create the job that ends Claude with zen: %w", err)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
		LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE}}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return errors.Join(fmt.Errorf("make the job end Claude when zen exits: %w", err), windows.CloseHandle(job))
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		return errors.Join(fmt.Errorf("put zen in the job that ends Claude with it: %w", err), windows.CloseHandle(job))
	}
	return nil
}

// parentExit reports once the shell that started zen exits, or the error that stopped zen waiting for it. Windows keeps
// a console program running when the shell that started it is killed, so zen ends Claude itself then.
func parentExit() (<-chan error, error) {
	parentPID := os.Getppid()
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(parentPID))
	if err != nil {
		return nil, fmt.Errorf("open zen's parent process %d to end Claude when it exits: %w", parentPID, err)
	}
	var parentStart, zenStart, exitTime, kernelTime, userTime windows.Filetime
	if err := windows.GetProcessTimes(parent, &parentStart, &exitTime, &kernelTime, &userTime); err != nil {
		return nil, errors.Join(fmt.Errorf("read when zen's parent process %d started: %w", parentPID, err), windows.CloseHandle(parent))
	}
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &zenStart, &exitTime, &kernelTime, &userTime); err != nil {
		return nil, errors.Join(fmt.Errorf("read when zen started: %w", err), windows.CloseHandle(parent))
	}
	if parentStart.Nanoseconds() > zenStart.Nanoseconds() {
		return nil, errors.Join(fmt.Errorf("zen's parent process %d exited before zen watched it, and its PID now names another process",
			parentPID), windows.CloseHandle(parent))
	}
	exited := make(chan error, 1)
	go func() {
		_, waitErr := windows.WaitForSingleObject(parent, windows.INFINITE)
		if err := errors.Join(waitErr, windows.CloseHandle(parent)); err != nil {
			exited <- fmt.Errorf("wait for zen's parent process %d to exit: %w", parentPID, err)
			return
		}
		exited <- nil
	}()
	return exited, nil
}
