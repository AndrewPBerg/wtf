package cli

import (
	"encoding/json"
	"io"
)

// jsonOutput is set by the global --json / -j flag.
var jsonOutput bool

// IsJSONOutput reports whether the global --json flag was set.
func IsJSONOutput() bool {
	return jsonOutput
}

// writeJSON encodes v as indented JSON to w.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
