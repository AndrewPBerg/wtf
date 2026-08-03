package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/AndrewPBerg/wtf/internal/notify"
	"github.com/AndrewPBerg/wtf/internal/watch"
	"github.com/spf13/cobra"
)

var (
	watchGlobal    bool
	watchInterval  int
	watchNoDesktop bool
)

func init() {
	watchCmd.Flags().BoolVarP(&watchGlobal, "global", "g", false, "Watch all registered repos")
	watchCmd.Flags().IntVarP(&watchInterval, "interval", "i", 0, "Poll interval in seconds (default 60)")
	watchCmd.Flags().BoolVar(&watchNoDesktop, "no-desktop", false, "Disable desktop notifications (terminal only)")
	rootCmd.AddCommand(watchCmd)
}

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch PRs for changes and send notifications",
	Long: `Watch pull requests for changes and send native desktop notifications.

Polls the forge API at a configurable interval and notifies on:
  • New PRs opened
  • PRs closed or merged
  • Review status changes (approved, changes requested)
  • Draft status changes

Examples:
  wtf watch              # watch current repo
  wtf watch -g           # watch all registered repos
  wtf watch -i 30        # poll every 30 seconds
  wtf watch --no-desktop # terminal-only notifications`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if watchGlobal {
			return runWatchGlobal(cmd)
		}
		return runWatchSingle(cmd)
	},
}

func runWatchSingle(cmd *cobra.Command) error {
	mgr, err := resolveManager(cmd)
	if err != nil {
		return err
	}

	dir, err := repoDirFor(mgr)
	if err != nil {
		return err
	}

	remoteURL, err := mgr.RemoteURL(dir)
	if err != nil {
		return fmt.Errorf("getting remote URL: %w", err)
	}

	f, err := forge.Detect(remoteURL, forge.WithTokenFunc(func() (string, error) {
		return tryToken(remoteURL)
	}))
	if err != nil {
		return fmt.Errorf("detecting forge: %w", err)
	}

	stateDir, err := mgr.StateDir(dir)
	if err != nil {
		return err
	}

	interval := resolveInterval(dir)
	notifier := resolveNotifier(dir)
	repoName := filepath.Base(dir)

	w := watch.New(f, notifier, stateDir,
		watch.WithInterval(interval),
		watch.WithRepoName(repoName),
		watch.WithRemoteURL(remoteURL),
		watch.WithLogger(cmd.ErrOrStderr()),
	)

	printBanner(cmd, repoName, interval, notifier)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return w.Run(ctx)
}

func runWatchGlobal(cmd *cobra.Command) error {
	repos, err := config.LoadValid()
	if err != nil {
		return fmt.Errorf("loading registered repos: %w", err)
	}

	if len(repos) == 0 {
		return fmt.Errorf("no registered repos — run wtf commands inside repos to register them")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s Watching %d repos for PR changes\n",
		cyanBold("wtf watch"),
		len(repos),
	)

	notifier := resolveNotifier("")

	var wg sync.WaitGroup
	for _, repo := range repos {
		wg.Add(1)
		go func(repoDir string) {
			defer wg.Done()
			if err := watchRepo(ctx, cmd, repoDir, notifier); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s [%s] %v\n",
					yellow("⚠"),
					filepath.Base(repoDir),
					err,
				)
			}
		}(repo)
	}

	wg.Wait()
	return nil
}

func watchRepo(ctx context.Context, cmd *cobra.Command, dir string, notifier notify.Notifier) error {
	mgrs := managersForRepo(dir)
	if len(mgrs) == 0 {
		return fmt.Errorf("could not determine the version control system for %s", dir)
	}
	// Watching tracks the forge, which is repo-wide, so either backend of a
	// colocated repo resolves the same remote.
	wm := mgrs[0]

	remoteURL, err := wm.RemoteURL(dir)
	if err != nil {
		return fmt.Errorf("getting remote URL: %w", err)
	}

	f, err := forge.Detect(remoteURL, forge.WithTokenFunc(func() (string, error) {
		return tryToken(remoteURL)
	}))
	if err != nil {
		return fmt.Errorf("detecting forge: %w", err)
	}

	stateDir, err := wm.StateDir(dir)
	if err != nil {
		return err
	}

	interval := resolveInterval(dir)
	repoName := filepath.Base(dir)

	w := watch.New(f, notifier, stateDir,
		watch.WithInterval(interval),
		watch.WithRepoName(repoName),
		watch.WithRemoteURL(remoteURL),
		watch.WithRepoColor(watch.ColorForRepo(repoName)),
		watch.WithLogger(cmd.ErrOrStderr()),
	)

	return w.Run(ctx)
}

// minInterval is the minimum allowed polling interval to avoid API rate limits.
const minInterval = 10 * time.Second

func resolveInterval(_ string) time.Duration {
	// CLI flag takes precedence.
	if watchInterval > 0 {
		d := time.Duration(watchInterval) * time.Second
		return clampInterval(d)
	}

	return watch.DefaultInterval
}

func clampInterval(d time.Duration) time.Duration {
	if d < minInterval {
		return minInterval
	}
	return d
}

func resolveNotifier(_ string) notify.Notifier {
	return notify.New(notify.WithTerminalOnly(watchNoDesktop))
}

func printBanner(cmd *cobra.Command, repoName string, interval time.Duration, n notify.Notifier) {
	w := cmd.ErrOrStderr()
	_, _ = fmt.Fprintf(w, "%s %s\n", cyanBold("wtf watch"), repoName)
	_, _ = fmt.Fprintf(w, "  Poll interval: %s | Notifications: %s\n",
		cyan(interval.String()),
		cyan(n.Name()),
	)

	var prefix string
	if strings.Contains(n.Name(), "desktop") {
		prefix = "Desktop + terminal"
	} else {
		prefix = "Terminal only"
	}
	_, _ = fmt.Fprintf(w, "  %s — press Ctrl+C to stop\n\n", dim(prefix))
}
