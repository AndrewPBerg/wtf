package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRerendererFirstRender(t *testing.T) {
	var buf bytes.Buffer
	r := NewRerenderer(&buf)

	r.Render("line1\nline2\n")

	out := buf.String()
	assert.Equal(t, "line1\nline2\n", out, "first render should have no ANSI prefix")
}

func TestRerendererSecondRender(t *testing.T) {
	var buf bytes.Buffer
	r := NewRerenderer(&buf)

	r.Render("old1\nold2\n")
	buf.Reset()

	r.Render("new1\nnew2\nnew3\n")

	out := buf.String()
	assert.True(t, strings.Contains(out, "\033[2A"), "should cursor-up 2 lines")
	assert.True(t, strings.Contains(out, "\033[J"), "should erase to end of screen")
	assert.True(t, strings.HasSuffix(out, "new1\nnew2\nnew3\n"), "should end with new content")
}

func TestRerendererLineCount(t *testing.T) {
	var buf bytes.Buffer
	r := NewRerenderer(&buf)

	r.Render("a\nb\nc\n")
	assert.Equal(t, 3, r.lineCount)

	buf.Reset()
	r.Render("x\n")

	out := buf.String()
	assert.True(t, strings.Contains(out, "\033[3A"), "should cursor-up 3 lines from previous render")
}

func TestRerendererNoNewlines(t *testing.T) {
	var buf bytes.Buffer
	r := NewRerenderer(&buf)

	r.Render("no newline")
	assert.Equal(t, 0, r.lineCount)

	buf.Reset()
	r.Render("next")
	// With 0 previous lines, no cursor-up should happen
	assert.Equal(t, "next", buf.String())
}
