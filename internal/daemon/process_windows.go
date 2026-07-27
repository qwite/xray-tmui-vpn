//go:build windows

package daemon

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/windows"
)

func detachedProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
		HideWindow:    true,
	}
}

func ignoreTerminalHangup() {}

func notifyStop(signals chan<- os.Signal) {
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
}

func terminateProcess(process *os.Process) error {
	return process.Kill()
}

func processRunning(process *os.Process) bool {
	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(process.Pid),
	)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == uint32(windows.STATUS_PENDING)
}
