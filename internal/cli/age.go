package cli

import (
	"fmt"
	"time"
)

// workspaceAge uses the largest whole unit; unknown registration times stay unknown.
func workspaceAge(createdAt *time.Time, now time.Time) string {
	if createdAt == nil || createdAt.IsZero() {
		return "?"
	}
	age := max(now.Sub(*createdAt), 0)
	for _, unit := range []struct {
		duration time.Duration
		suffix   string
	}{
		{7 * 24 * time.Hour, "W"},
		{24 * time.Hour, "D"},
		{time.Hour, "H"},
		{time.Minute, "M"},
		{time.Second, "S"},
	} {
		if age >= unit.duration {
			return fmt.Sprintf("%d%s", age/unit.duration, unit.suffix)
		}
	}
	return "0S"
}
