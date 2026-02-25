package ui

import (
	"fmt"
)

// StartFrame prints a dimmed separator header: "  --- title ---"
func (u *UI) StartFrame(title string) {
	line := fmt.Sprintf("  --- %s ---", title)
	if u.isTTY {
		style := u.renderer.NewStyle().Faint(true)
		u.println(style.Render(line))
	} else {
		u.println(line)
	}
}

// EndFrame prints a dimmed closing separator: "  ---"
func (u *UI) EndFrame() {
	line := "  ---"
	if u.isTTY {
		style := u.renderer.NewStyle().Faint(true)
		u.println(style.Render(line))
	} else {
		u.println(line)
	}
}
