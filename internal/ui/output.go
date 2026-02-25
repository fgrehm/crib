package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Header prints a section header: "==> msg" in bold blue.
func (u *UI) Header(msg string) {
	if u.isTTY {
		style := u.renderer.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
		u.println(style.Render("==> " + msg))
	} else {
		u.println("==> " + msg)
	}
}

// Success prints a success message: "  ✓ msg" in green (TTY) or "  ok msg" (non-TTY).
func (u *UI) Success(msg string) {
	if u.isTTY {
		style := u.renderer.NewStyle().Foreground(lipgloss.Color("2"))
		u.println(style.Render("  ✓ " + msg))
	} else {
		u.println("  ok " + msg)
	}
}

// Keyval prints a label-value pair: "  label   value" with bold fixed-width label.
func (u *UI) Keyval(key, value string) {
	padded := fmt.Sprintf("%-12s", key)
	if u.isTTY {
		style := u.renderer.NewStyle().Bold(true)
		u.printf("  %s%s\n", style.Render(padded), value)
	} else {
		u.printf("  %s%s\n", padded, value)
	}
}

// Dim prints dimmed text.
func (u *UI) Dim(msg string) {
	if u.isTTY {
		style := u.renderer.NewStyle().Faint(true)
		u.println(style.Render(msg))
	} else {
		u.println(msg)
	}
}

// Error prints an error message: "error: msg" to errOut.
// Only the "error:" prefix is styled to prevent lipgloss from mangling
// multi-line message bodies.
func (u *UI) Error(msg string) {
	if u.isTTY {
		prefix := u.renderer.NewStyle().Foreground(lipgloss.Color("1")).Render("error:")
		_, _ = fmt.Fprintf(u.errOut, "%s %s\n", prefix, msg)
	} else {
		_, _ = fmt.Fprintln(u.errOut, "error: "+msg)
	}
}

// StatusColor returns the status string colored green if "running", yellow otherwise.
func (u *UI) StatusColor(status string) string {
	if !u.isTTY {
		return status
	}
	if strings.EqualFold(status, "running") {
		return u.renderer.NewStyle().Foreground(lipgloss.Color("2")).Render(status)
	}
	return u.renderer.NewStyle().Foreground(lipgloss.Color("3")).Render(status)
}

// Table prints a column-aligned table with bold headers.
func (u *UI) Table(headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}

	// Calculate column widths.
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print headers.
	var hdr strings.Builder
	for i, h := range headers {
		if i > 0 {
			hdr.WriteString("  ")
		}
		fmt.Fprintf(&hdr, "%-*s", widths[i], h)
	}
	if u.isTTY {
		style := u.renderer.NewStyle().Bold(true)
		u.println(style.Render(hdr.String()))
	} else {
		u.println(hdr.String())
	}

	// Print rows.
	for _, row := range rows {
		var line strings.Builder
		for i, cell := range row {
			if i > 0 {
				line.WriteString("  ")
			}
			if i < len(widths) {
				fmt.Fprintf(&line, "%-*s", widths[i], cell)
			} else {
				line.WriteString(cell)
			}
		}
		u.println(line.String())
	}
}

// println writes a line to out, discarding errors (not recoverable in CLI output).
func (u *UI) println(msg string) {
	_, _ = fmt.Fprintln(u.out, msg)
}

// printf writes formatted output to out, discarding errors.
func (u *UI) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(u.out, format, args...)
}
