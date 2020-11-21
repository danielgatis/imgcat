// +build windows

package main

import (
	"log"
	"os"

	"golang.org/x/sys/windows"
)

func disableEcho() uint32 {
	var st uint32

	if err := windows.GetConsoleMode(windows.Handle(os.Stdout.Fd()), &st); err != nil {
		log.Fatalf("failed to get the console state: %v", err)
	}

	newSt := st
	newSt = newSt &^ windows.ENABLE_ECHO_INPUT
	newSt |= windows.ENABLE_PROCESSED_INPUT
	newSt |= windows.ENABLE_LINE_INPUT
	newSt |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING

	if err := windows.SetConsoleMode(windows.Handle(os.Stdout.Fd()), newSt); err != nil {
		log.Fatalf("failed to set the console state: %v", err)
	}

	return st
}

func enableEcho(st uint32) {
	if err := windows.SetConsoleMode(windows.Handle(os.Stdout.Fd()), st); err != nil {
		log.Fatalf("failed to set the console state: %v", err)
	}
}
