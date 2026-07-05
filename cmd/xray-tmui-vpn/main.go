package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/qwites/xray-tmui-vpn/internal/tui"
)

func main() {
	program := tea.NewProgram(tui.NewModel(), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "xray-tmui-vpn: %v\n", err)
		os.Exit(1)
	}
}
