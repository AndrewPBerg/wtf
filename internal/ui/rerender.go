package ui

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// Rerenderer tracks a block of terminal output and can overwrite it in-place
// using ANSI escape sequences. This enables a "lazy loading" pattern where
// initial output is displayed immediately and then replaced when updated
// data becomes available.
type Rerenderer struct {
	out       io.Writer
	lineCount int
	mu        sync.Mutex
}

// NewRerenderer returns a Rerenderer that writes to out.
func NewRerenderer(out io.Writer) *Rerenderer {
	return &Rerenderer{out: out}
}

// Render writes content to the output. On subsequent calls, it moves the
// cursor up to overwrite the previously rendered block before writing.
func (r *Rerenderer) Render(content string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.lineCount > 0 {
		// Move cursor up to the start of the previously rendered block
		_, _ = fmt.Fprintf(r.out, "\033[%dA", r.lineCount)
		// Erase from cursor to end of screen
		_, _ = fmt.Fprint(r.out, "\033[J")
	}

	_, _ = fmt.Fprint(r.out, content)
	r.lineCount = strings.Count(content, "\n")
}
