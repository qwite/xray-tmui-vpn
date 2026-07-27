//go:build !windows

package daemon

import (
	"os"
	"os/signal"
	"syscall"
)

func detachedProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func ignoreTerminalHangup() {
	signal.Ignore(syscall.SIGHUP)
}

func notifyStop(signals chan<- os.Signal) {
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
}

func terminateProcess(process *os.Process) error {
	return process.Kill()
}

func processRunning(process *os.Process) bool {
	return process.Signal(syscall.Signal(0)) == nil
}
