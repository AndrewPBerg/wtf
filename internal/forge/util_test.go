package forge

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadBody(t *testing.T) {
	data := []byte("hello world")
	body, err := readBody(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, data, body)
}

func TestReadBody_Empty(t *testing.T) {
	body, err := readBody(bytes.NewReader(nil))
	require.NoError(t, err)
	assert.Empty(t, body)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestReadBody_Error(t *testing.T) {
	_, err := readBody(errReader{})
	assert.Error(t, err)
}
