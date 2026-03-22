package watch

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/AndrewPBerg/wtf/internal/notify"
	"github.com/fatih/color"
)

// DefaultInterval is the default polling interval.
const DefaultInterval = 60 * time.Second

// repoColors is a palette of distinct colors for differentiating repos in global mode.
var repoColors = []color.Attribute{
	color.FgCyan,
	color.FgGreen,
	color.FgMagenta,
	color.FgYellow,
	color.FgBlue,
	color.FgHiCyan,
	color.FgHiGreen,
	color.FgHiMagenta,
}

// Watcher polls a forge for PR changes and sends notifications.
type Watcher struct {
	forge     forge.Forge
	notifier  notify.Notifier
	stateDir  string
	interval  time.Duration
	repoName  string
	repoColor color.Attribute
	logger    io.Writer
	pollErr   bool // tracks whether we're in an error state (to avoid log spam)
}

// Option configures a Watcher.
type Option func(*Watcher)

// WithInterval sets the polling interval.
func WithInterval(d time.Duration) Option {
	return func(w *Watcher) {
		w.interval = d
	}
}

// WithLogger sets the log output writer.
func WithLogger(out io.Writer) Option {
	return func(w *Watcher) {
		w.logger = out
	}
}

// WithRepoName sets the display name for notifications.
func WithRepoName(name string) Option {
	return func(w *Watcher) {
		w.repoName = name
	}
}

// WithRepoColor sets the color used for the repo name prefix in log output.
func WithRepoColor(c color.Attribute) Option {
	return func(w *Watcher) {
		w.repoColor = c
	}
}

// ColorForRepo returns a deterministic color for a repo name.
func ColorForRepo(name string) color.Attribute {
	var h uint32
	for _, c := range name {
		h = h*31 + uint32(c)
	}
	return repoColors[h%uint32(len(repoColors))]
}

// New creates a Watcher for a single repository.
func New(f forge.Forge, n notify.Notifier, stateDir string, opts ...Option) *Watcher {
	w := &Watcher{
		forge:    f,
		notifier: n,
		stateDir: stateDir,
		interval: DefaultInterval,
		logger:   os.Stderr,
	}
	for _, o := range opts {
		o(w)
	}
	return w
}

// Run polls for PR changes until ctx is cancelled.
// On first run, it populates state silently without firing notifications.
func (w *Watcher) Run(ctx context.Context) error {
	state, err := LoadState(w.stateDir)
	if err != nil {
		return fmt.Errorf("loading watch state: %w", err)
	}

	// First poll: populate state silently.
	if state.IsFirstRun() {
		prs, err := w.forge.ListPRs(ctx)
		if err != nil {
			return fmt.Errorf("initial PR fetch: %w", err)
		}
		state = SnapshotPRs(prs)
		if err := SaveState(w.stateDir, state); err != nil {
			return fmt.Errorf("saving initial state: %w", err)
		}
		w.logf("Watching %d open PRs", len(prs))
	} else {
		w.logf("Resumed with %d tracked PRs", len(state.PRs))
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			state, err = w.poll(ctx, state)
			if err != nil {
				// Only log the first error to avoid spamming on persistent failures.
				if !w.pollErr {
					w.logf("poll error: %v", err)
					w.pollErr = true
				}
			} else if w.pollErr {
				w.logf("poll recovered")
				w.pollErr = false
			}
		}
	}
}

func (w *Watcher) poll(ctx context.Context, state State) (State, error) {
	prs, err := w.forge.ListPRs(ctx)
	if err != nil {
		return state, fmt.Errorf("fetching PRs: %w", err)
	}

	events, disappeared := Diff(state, prs)

	// Look up disappeared PRs to classify as merged vs closed.
	for _, num := range disappeared {
		pr, err := w.forge.GetPR(ctx, num)
		if err != nil {
			// If we can't look it up, report as closed.
			snap := state.PRs[num]
			events = append(events, Event{
				Kind:   EventPRClosed,
				PR:     forge.PR{Number: num, Title: snap.Title},
				Detail: "closed (lookup failed)",
			})
			continue
		}
		events = append(events, ClassifyDisappeared(*pr))
	}

	// Send notifications for all events.
	for _, e := range events {
		if w.repoName != "" {
			e.Repo = w.repoName
		}
		w.logEvent(e)
		title := "wtf"
		if w.repoName != "" {
			title = fmt.Sprintf("wtf · %s", w.repoName)
		}
		if err := w.notifier.Notify(ctx, title, e.String()); err != nil {
			w.logf("notification error: %v", err)
		}
	}

	// Update state.
	newState := SnapshotPRs(prs)
	if err := SaveState(w.stateDir, newState); err != nil {
		return newState, fmt.Errorf("saving state: %w", err)
	}

	return newState, nil
}

func (w *Watcher) logf(format string, args ...any) {
	w.writeLog(fmt.Sprintf(format, args...))
}

func (w *Watcher) logEvent(e Event) {
	prefix := ""
	if e.Repo != "" {
		c := w.repoColor
		if c == 0 {
			c = color.FgCyan
		}
		prefix = color.New(c).Sprintf("[%s] ", e.Repo)
	}
	msg := e.String()
	if e.PR.URL != "" {
		msg += " " + color.New(color.Faint).Sprint(e.PR.URL)
	}
	w.writeLog(prefix + msg)
}

func (w *Watcher) writeLog(msg string) {
	timestamp := color.New(color.Faint).Sprint(time.Now().Format("15:04:05"))
	_, _ = fmt.Fprintf(w.logger, "%s %s\n", timestamp, msg)
}
