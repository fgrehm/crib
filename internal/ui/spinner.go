package ui

import (
	"fmt"
	"time"
)

var brailleFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner displays an animated progress indicator.
type Spinner struct {
	done    chan struct{}
	stopped chan struct{}
}

// StartSpinner begins an animated spinner with the given message.
// In non-TTY mode it prints the message once and returns immediately.
// Call Stop() to clear the spinner line.
func (u *UI) StartSpinner(msg string) *Spinner {
	if !u.isTTY {
		u.printf("  %s...\n", msg)
		s := &Spinner{
			done:    make(chan struct{}),
			stopped: make(chan struct{}),
		}
		close(s.stopped)
		return s
	}

	s := &Spinner{
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
	}

	go func() {
		defer close(s.stopped)
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		i := 0
		for {
			select {
			case <-s.done:
				// Clear the spinner line.
				_, _ = fmt.Fprintf(u.out, "\r\033[K")
				return
			case <-ticker.C:
				frame := brailleFrames[i%len(brailleFrames)]
				_, _ = fmt.Fprintf(u.out, "\r  %s %s", frame, msg)
				i++
			}
		}
	}()

	return s
}

// Stop halts the spinner and clears its line.
func (s *Spinner) Stop() {
	select {
	case <-s.done:
		// Already stopped.
	default:
		close(s.done)
	}
	<-s.stopped
}
