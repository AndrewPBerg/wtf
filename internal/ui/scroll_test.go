package ui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewScrollWriter_DefaultHeight(t *testing.T) {
	sw := NewScrollWriter(&bytes.Buffer{}, 0)
	assert.Equal(t, DefaultScrollHeight, sw.maxLines)
}

func TestNewScrollWriter_CustomHeight(t *testing.T) {
	sw := NewScrollWriter(&bytes.Buffer{}, 5)
	assert.Equal(t, 5, sw.maxLines)
}

func TestScrollWriter_FewLines_NoScrolling(t *testing.T) {
	var buf bytes.Buffer
	sw := NewScrollWriter(&buf, 10)

	for i := 1; i <= 3; i++ {
		_, _ = fmt.Fprintf(sw, "line %d\n", i)
	}

	// With only 3 lines (< maxLines), the final output should just show
	// each line printed in sequence. After the first write there's no cursor
	// movement needed, but subsequent writes move up to redraw.
	assert.Equal(t, 3, sw.Lines())
}

func TestScrollWriter_ExactMaxLines(t *testing.T) {
	var buf bytes.Buffer
	sw := NewScrollWriter(&buf, 3)

	_, _ = fmt.Fprint(sw, "a\nb\nc\n")

	assert.Equal(t, 3, sw.Lines())
	output := buf.String()
	// All three lines should be visible
	assert.Contains(t, output, "a\n")
	assert.Contains(t, output, "b\n")
	assert.Contains(t, output, "c\n")
}

func TestScrollWriter_ScrollsBeyondMax(t *testing.T) {
	var buf bytes.Buffer
	sw := NewScrollWriter(&buf, 3)

	for i := 1; i <= 5; i++ {
		_, _ = fmt.Fprintf(sw, "line %d\n", i)
	}

	assert.Equal(t, 5, sw.Lines())

	// The final visible lines should be 3, 4, 5 (last 3)
	// Extract the final state by finding the last redraw
	output := buf.String()
	lastLine5 := strings.LastIndex(output, "line 5")
	require.NotEqual(t, -1, lastLine5, "line 5 should be in output")

	// "line 1" should have appeared initially but be scrolled off
	// (it will still be in the buffer due to ANSI redraws)
	assert.Contains(t, output, "line 1")
}

func TestScrollWriter_Flush_PartialLine(t *testing.T) {
	var buf bytes.Buffer
	sw := NewScrollWriter(&buf, 10)

	_, _ = fmt.Fprint(sw, "complete\n")
	_, _ = fmt.Fprint(sw, "partial with no newline")

	assert.Equal(t, 1, sw.Lines())

	sw.Flush()

	assert.Equal(t, 2, sw.Lines())
}

func TestScrollWriter_HandlesCRLF(t *testing.T) {
	var buf bytes.Buffer
	sw := NewScrollWriter(&buf, 10)

	_, _ = fmt.Fprint(sw, "windows line\r\n")

	assert.Equal(t, 1, sw.Lines())
	// The \r should be trimmed
	assert.Equal(t, "windows line", sw.lines[0])
}

func TestScrollWriter_WriteReturnsByteCount(t *testing.T) {
	var buf bytes.Buffer
	sw := NewScrollWriter(&buf, 10)

	input := []byte("hello world\nfoo\n")
	n, err := sw.Write(input)

	require.NoError(t, err)
	assert.Equal(t, len(input), n)
}

func TestScrollWriter_EmptyWrite(t *testing.T) {
	var buf bytes.Buffer
	sw := NewScrollWriter(&buf, 10)

	n, err := sw.Write([]byte{})
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, sw.Lines())
}

func TestScrollWriter_MultiplePartialWrites(t *testing.T) {
	var buf bytes.Buffer
	sw := NewScrollWriter(&buf, 10)

	// Simulate chunked output like a subprocess might produce
	_, _ = fmt.Fprint(sw, "hel")
	_, _ = fmt.Fprint(sw, "lo\nwor")
	_, _ = fmt.Fprint(sw, "ld\n")

	assert.Equal(t, 2, sw.Lines())
	assert.Equal(t, "hello", sw.lines[0])
	assert.Equal(t, "world", sw.lines[1])
}

func TestScrollWriter_LargeScroll(t *testing.T) {
	var buf bytes.Buffer
	sw := NewScrollWriter(&buf, 5)

	for i := 1; i <= 100; i++ {
		_, _ = fmt.Fprintf(sw, "line %d\n", i)
	}

	assert.Equal(t, 100, sw.Lines())
	assert.Equal(t, 5, sw.rendered)
}

func TestScrollWriter_FlushNoPartial(t *testing.T) {
	var buf bytes.Buffer
	sw := NewScrollWriter(&buf, 10)

	_, _ = fmt.Fprint(sw, "complete\n")
	linesBefore := sw.Lines()

	sw.Flush()

	// No partial data, so line count should not change
	assert.Equal(t, linesBefore, sw.Lines())
}

func TestScrollWriter_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	sw := NewScrollWriter(&buf, 10)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				_, _ = fmt.Fprintf(sw, "goroutine %d line %d\n", n, j)
			}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	sw.Flush()
	assert.Equal(t, 500, sw.Lines())
}
