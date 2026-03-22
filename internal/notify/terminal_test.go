package notify

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerminalNotifier_Name(t *testing.T) {
	n := newTerminal(&bytes.Buffer{})
	assert.Equal(t, "terminal", n.Name())
}

func TestTerminalNotifier_Notify(t *testing.T) {
	tests := []struct {
		name  string
		title string
		body  string
		want  string
	}{
		{
			name:  "simple notification",
			title: "WTF",
			body:  "PR #42 approved",
			want:  "PR #42 approved",
		},
		{
			name:  "contains BEL character",
			title: "Alert",
			body:  "new PR",
			want:  "\a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			n := newTerminal(&buf)

			err := n.Notify(context.Background(), tt.title, tt.body)
			require.NoError(t, err)
			assert.Contains(t, buf.String(), tt.want)
		})
	}
}

func TestTerminalNotifier_ContainsTitleAndBody(t *testing.T) {
	var buf bytes.Buffer
	n := newTerminal(&buf)

	err := n.Notify(context.Background(), "MyTitle", "my body text")
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "MyTitle")
	assert.Contains(t, output, "my body text")
}
