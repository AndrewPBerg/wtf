package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionCommand(t *testing.T) {
	saved := jsonOutput
	defer func() { jsonOutput = saved }()
	jsonOutput = false

	buf := new(bytes.Buffer)
	versionCmd.SetOut(buf)

	err := versionCmd.RunE(versionCmd, nil)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "wtf version")
}
