package ui

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
)

// DefaultScrollHeight is the default number of visible lines in the scroll view.
const DefaultScrollHeight = 10

// ScrollWriter wraps an io.Writer and constrains output to a fixed-height
// scrolling region. When output exceeds the configured height, earlier lines
// scroll off the top and only the most recent lines remain visible.
type ScrollWriter struct {
	out      io.Writer
	maxLines int
	lines    []string
	partial  []byte
	rendered int
	mu       sync.Mutex
}

// NewScrollWriter returns a ScrollWriter that displays at most maxLines of
// output at a time. If maxLines <= 0, DefaultScrollHeight is used.
func NewScrollWriter(out io.Writer, maxLines int) *ScrollWriter {
	if maxLines <= 0 {
		maxLines = DefaultScrollHeight
	}
	return &ScrollWriter{
		out:      out,
		maxLines: maxLines,
	}
}

// Write implements io.Writer. It buffers incoming bytes, splits on newlines,
// and redraws the visible scroll region.
func (sw *ScrollWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	sw.partial = append(sw.partial, p...)

	for {
		idx := bytes.IndexByte(sw.partial, '\n')
		if idx == -1 {
			break
		}
		line := string(sw.partial[:idx])
		line = strings.TrimRight(line, "\r")
		sw.lines = append(sw.lines, line)
		sw.partial = sw.partial[idx+1:]
	}

	sw.redraw()
	return len(p), nil
}

// Flush writes any remaining partial line and performs a final redraw.
func (sw *ScrollWriter) Flush() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.partial) > 0 {
		sw.lines = append(sw.lines, string(sw.partial))
		sw.partial = sw.partial[:0]
	}

	sw.redraw()
}

// Lines returns the total number of complete lines received so far.
func (sw *ScrollWriter) Lines() int {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return len(sw.lines)
}

func (sw *ScrollWriter) redraw() {
	start := 0
	if len(sw.lines) > sw.maxLines {
		start = len(sw.lines) - sw.maxLines
	}
	visible := sw.lines[start:]

	if sw.rendered > 0 {
		// Move cursor up to the start of the previously rendered block
		_, _ = fmt.Fprintf(sw.out, "\033[%dA", sw.rendered)
		// Erase from cursor to end of screen
		_, _ = fmt.Fprint(sw.out, "\033[J")
	}

	for _, line := range visible {
		_, _ = fmt.Fprintln(sw.out, line)
	}

	sw.rendered = len(visible)
}
