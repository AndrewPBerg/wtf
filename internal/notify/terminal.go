package notify

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/fatih/color"
)

// terminalNotifier writes notifications to a terminal via BEL + colored output.
// It is safe for concurrent use from multiple goroutines.
type terminalNotifier struct {
	mu sync.Mutex
	w  io.Writer
}

func newTerminal(w io.Writer) *terminalNotifier {
	return &terminalNotifier{w: w}
}

func (t *terminalNotifier) Name() string { return "terminal" }

func (t *terminalNotifier) Notify(_ context.Context, title, body string) error {
	bold := color.New(color.Bold)
	line := fmt.Sprintf("\a%s %s\n", bold.Sprint(title+":"), body)
	t.mu.Lock()
	defer t.mu.Unlock()
	_, err := fmt.Fprint(t.w, line)
	return err
}
