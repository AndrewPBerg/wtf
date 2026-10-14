package core

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"time"
)

// invokeReviewAdapter owns the shared bridge lifecycle for single reads,
// batches and doctor. Only structured stdout reaches the caller.
func invokeReviewAdapter(ctx context.Context, root string, arguments []string, input io.Reader) ([]byte, string, error) {
	if ctx.Err() != nil {
		return nil, "review_lookup_cancelled", ctx.Err()
	}
	bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	node, err := doctorBinary(bounded, root, "node")
	if err != nil {
		return nil, "node_missing", err
	}
	file, err := os.CreateTemp("", "wtf-review-*.mjs")
	if err != nil {
		return nil, "review_adapter_unavailable", err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	_, writeErr := file.Write(reviewBridgeSource)
	closeErr := file.Close()
	if writeErr != nil {
		return nil, "review_adapter_unavailable", writeErr
	}
	if closeErr != nil {
		return nil, "review_adapter_unavailable", closeErr
	}
	args := append([]string{file.Name(), "--repo", root, "--timeout-ms", "5000"}, arguments...)
	cmd := exec.CommandContext(bounded, node, args...)
	cmd.Dir, cmd.Stdin = root, input
	var data bytes.Buffer
	cmd.Stdout, cmd.Stderr = &data, io.Discard
	configureCommand(cmd)
	runErr := cmd.Run()
	finishCommand(cmd)
	if bounded.Err() != nil {
		if ctx.Err() == context.Canceled {
			return nil, "review_lookup_cancelled", runErr
		}
		return nil, "linear_lookup_timeout", runErr
	}
	return data.Bytes(), "", runErr
}
