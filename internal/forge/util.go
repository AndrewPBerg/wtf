package forge

import (
	"fmt"
	"io"
)

const maxBodySize = 10 * 1024 * 1024 // 10MB

// readBody reads an HTTP response body with a size limit.
func readBody(r io.Reader) ([]byte, error) {
	limited := io.LimitReader(r, maxBodySize)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	return data, nil
}
