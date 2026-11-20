package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func MakeConsoleRaw() (func() error, error) {
	input := windows.Handle(os.Stdin.Fd())
	output := windows.Handle(os.Stdout.Fd())
	var inputMode, outputMode uint32
	if err := windows.GetConsoleMode(input, &inputMode); err != nil {
		return nil, fmt.Errorf("GetConsoleMode stdin: %w", err)
	}
	if err := windows.GetConsoleMode(output, &outputMode); err != nil {
		return nil, fmt.Errorf("GetConsoleMode stdout: %w", err)
	}
	rawInputMode := inputMode&^(windows.ENABLE_ECHO_INPUT|windows.ENABLE_LINE_INPUT|windows.ENABLE_PROCESSED_INPUT) |
		windows.ENABLE_VIRTUAL_TERMINAL_INPUT
	if err := windows.SetConsoleMode(input, rawInputMode); err != nil {
		return nil, fmt.Errorf("SetConsoleMode stdin %#x: %w", rawInputMode, err)
	}
	vtOutputMode := outputMode | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	if err := windows.SetConsoleMode(output, vtOutputMode); err != nil {
		return nil, fmt.Errorf("SetConsoleMode stdout %#x: %w", vtOutputMode, err)
	}
	return func() error {
		if err := windows.SetConsoleMode(input, inputMode); err != nil {
			return fmt.Errorf("restore stdin console mode %#x: %w", inputMode, err)
		}
		if err := windows.SetConsoleMode(output, outputMode); err != nil {
			return fmt.Errorf("restore stdout console mode %#x: %w", outputMode, err)
		}
		return nil
	}, nil
}

func ConsoleColumns() (int, error) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(os.Stdout.Fd()), &info); err != nil {
		return 0, fmt.Errorf("GetConsoleScreenBufferInfo stdout: %w", err)
	}
	return int(info.Window.Right-info.Window.Left) + 1, nil
}
