package git

import "fmt"

// mockExecutor is a test double for Executor.
type mockExecutor struct {
	// responses maps "arg0 arg1 ..." to (output, error)
	responses map[string]mockResponse
}

type mockResponse struct {
	output string
	err    error
}

func newMockExecutor() *mockExecutor {
	return &mockExecutor{responses: make(map[string]mockResponse)}
}

func (m *mockExecutor) on(args string, output string, err error) {
	m.responses[args] = mockResponse{output: output, err: err}
}

func (m *mockExecutor) Run(_ string, args ...string) (string, error) {
	key := ""
	for i, a := range args {
		if i > 0 {
			key += " "
		}
		key += a
	}

	resp, ok := m.responses[key]
	if !ok {
		return "", fmt.Errorf("unexpected call: git %s", key)
	}
	return resp.output, resp.err
}
