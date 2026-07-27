package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/qwites/xray-tmui-vpn/internal/buildinfo"
	"github.com/qwites/xray-tmui-vpn/internal/daemon"
	"github.com/qwites/xray-tmui-vpn/internal/tui"
)

func main() {
	if versionRequested(os.Args[1:]) {
		fmt.Println(buildinfo.String())
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "daemon" {
		if err := daemon.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "xray-tmui-vpn daemon: %v\n", err)
			os.Exit(1)
		}
		return
	}

	program := tea.NewProgram(tui.NewModel(), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "xray-tmui-vpn: %v\n", err)
		os.Exit(1)
	}
}

func versionRequested(args []string) bool {
	if len(args) != 1 {
		return false
	}
	return args[0] == "--version" || args[0] == "version"
}
