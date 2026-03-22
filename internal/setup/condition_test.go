package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvalCondition_Empty(t *testing.T) {
	result, err := EvalCondition("", &ConditionContext{})
	require.NoError(t, err)
	assert.True(t, result)
}

func TestEvalCondition_Whitespace(t *testing.T) {
	result, err := EvalCondition("   ", &ConditionContext{})
	require.NoError(t, err)
	assert.True(t, result)
}

func TestEvalCondition_BranchContains(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		branch   string
		expected bool
	}{
		{"match", "branch contains 'feature'", "feature/auth", true},
		{"no match", "branch contains 'feature'", "bugfix/auth", false},
		{"exact match", "branch contains 'main'", "main", true},
		{"substring match", "branch contains 'fix'", "bugfix/auth", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &ConditionContext{Branch: tt.branch}
			result, err := EvalCondition(tt.expr, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEvalCondition_BranchContains_InvalidQuoting(t *testing.T) {
	ctx := &ConditionContext{Branch: "main"}
	_, err := EvalCondition("branch contains feature", ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected quoted value")
}

func TestEvalCondition_FileExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "exists.txt"), []byte("hi"), 0o644))

	ctx := &ConditionContext{Dir: dir}

	result, err := EvalCondition("file exists 'exists.txt'", ctx)
	require.NoError(t, err)
	assert.True(t, result)

	result, err = EvalCondition("file exists 'nope.txt'", ctx)
	require.NoError(t, err)
	assert.False(t, result)
}

func TestEvalCondition_FileExists_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "abs.txt")
	require.NoError(t, os.WriteFile(path, []byte("hi"), 0o644))

	ctx := &ConditionContext{Dir: "/some/other/dir"}

	result, err := EvalCondition("file exists '"+path+"'", ctx)
	require.NoError(t, err)
	assert.True(t, result)
}

func TestEvalCondition_FileExists_InvalidQuoting(t *testing.T) {
	ctx := &ConditionContext{Dir: "/tmp"}
	_, err := EvalCondition("file exists nopath", ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected quoted path")
}

func TestEvalCondition_EnvIsSet(t *testing.T) {
	envVars := map[string]string{
		"MY_VAR": "hello",
	}
	ctx := &ConditionContext{
		GetEnv: func(key string) string { return envVars[key] },
	}

	result, err := EvalCondition("env MY_VAR is set", ctx)
	require.NoError(t, err)
	assert.True(t, result)

	result, err = EvalCondition("env MISSING_VAR is set", ctx)
	require.NoError(t, err)
	assert.False(t, result)
}

func TestEvalCondition_EnvIsSet_NilGetEnv(t *testing.T) {
	ctx := &ConditionContext{}
	// Uses os.Getenv as fallback — test with a known-unset var
	result, err := EvalCondition("env WTF_TEST_UNLIKELY_VAR_NAME is set", ctx)
	require.NoError(t, err)
	assert.False(t, result)
}

func TestEvalCondition_EnvIsSet_EmptyVarName(t *testing.T) {
	ctx := &ConditionContext{}
	_, err := EvalCondition("env  is set", ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing variable name")
}

func TestEvalCondition_UnknownCondition(t *testing.T) {
	ctx := &ConditionContext{}
	_, err := EvalCondition("something weird", ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown condition")
}

func TestExtractQuoted(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"'hello'", "hello"},
		{"  'hello'  ", "hello"},
		{"hello", ""},
		{"'hello", ""},
		{"hello'", ""},
		{"''", ""},
		{"'a'", "a"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractQuoted(tt.input))
		})
	}
}
