package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConditionContext provides context for condition evaluation.
type ConditionContext struct {
	Branch string
	Dir    string
	GetEnv func(string) string
}

// EvalCondition evaluates a condition expression.
// Supported forms:
//   - "" (empty) → true
//   - "branch contains '<value>'"
//   - "file exists '<path>'"
//   - "env VAR is set"
func EvalCondition(expr string, ctx *ConditionContext) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}

	if strings.HasPrefix(expr, "branch contains ") {
		val := extractQuoted(strings.TrimPrefix(expr, "branch contains "))
		if val == "" {
			return false, fmt.Errorf("invalid condition: %q (expected quoted value)", expr)
		}
		return strings.Contains(ctx.Branch, val), nil
	}

	if strings.HasPrefix(expr, "file exists ") {
		val := extractQuoted(strings.TrimPrefix(expr, "file exists "))
		if val == "" {
			return false, fmt.Errorf("invalid condition: %q (expected quoted path)", expr)
		}
		path := val
		if !filepath.IsAbs(path) {
			path = filepath.Join(ctx.Dir, path)
		}
		_, err := os.Stat(path)
		return err == nil, nil
	}

	if strings.HasPrefix(expr, "env ") && strings.HasSuffix(expr, " is set") {
		varName := strings.TrimSuffix(strings.TrimPrefix(expr, "env "), " is set")
		varName = strings.TrimSpace(varName)
		if varName == "" {
			return false, fmt.Errorf("invalid condition: %q (missing variable name)", expr)
		}
		getEnv := ctx.GetEnv
		if getEnv == nil {
			getEnv = os.Getenv
		}
		return getEnv(varName) != "", nil
	}

	return false, fmt.Errorf("unknown condition: %q", expr)
}

// extractQuoted extracts a value between single quotes.
func extractQuoted(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return ""
}
