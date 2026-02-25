package ui

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

// UI provides styled terminal output.
type UI struct {
	out      io.Writer
	errOut   io.Writer
	isTTY    bool
	renderer *lipgloss.Renderer
}

// New creates a UI that writes to out and errOut.
// TTY detection is performed on out.
func New(out, errOut io.Writer) *UI {
	tty := false
	if f, ok := out.(*os.File); ok {
		tty = term.IsTerminal(f.Fd())
	}
	return &UI{
		out:      out,
		errOut:   errOut,
		isTTY:    tty,
		renderer: lipgloss.NewRenderer(out),
	}
}

// IsTTY reports whether the output is a terminal.
func (u *UI) IsTTY() bool {
	return u.isTTY
}
