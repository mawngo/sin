package core

import (
	"context"
	"fmt"
	"github.com/flc1125/go-cron/v4"
	"github.com/pterm/pterm"
	"log/slog"
	"time"
)

// Run execute the function with given frequency without overlapping.
// Run stop if the function returns an error.
func Run(ctx context.Context, freq string, fn func() error) error {
	if freq == "" {
		return fn()
	}

	if dur, err := time.ParseDuration(freq); err == nil {
		return runInterval(ctx, dur, fn)
	}

	return runCron(ctx, freq, fn)
}

func runInterval(ctx context.Context, dur time.Duration, fn func() error) error {
	timer := time.NewTimer(dur)
	if err := fn(); err != nil {
		return err
	}
	for {
		startWait := time.Now()
		select {
		case <-timer.C:
			if time.Since(startWait) < 10*time.Second {
				pterm.Warning.Println("Sync can't keep up with the frequency")
				pterm.Warning.Println("Sync job take too long or the frequency is too fast")
				slog.Warn("Slow sync process", slog.String("freq", dur.String()))
			}
			timer = time.NewTimer(dur)
			if err := fn(); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func runCron(ctx context.Context, freq string, fn func() error) error {
	c := cron.New(
		cron.WithContext(ctx),
		cron.WithLogger(cron.DiscardLogger),
	)
	defer c.Stop()

	jobs := make(chan struct{}, 1)
	_, err := c.AddFunc(freq, func(ctx context.Context) error {
		select {
		case jobs <- struct{}{}:
		case <-ctx.Done():
		default:
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("invalid cron expression [%s]: %w", freq, err)
	}

	c.Start()
	for {
		startWait := time.Now()
		select {
		case <-jobs:
			if time.Since(startWait) < 10*time.Second {
				pterm.Warning.Println("Sync can't keep up with the frequency")
				pterm.Warning.Println("Sync job take too long or the frequency is too fast")
				slog.Warn("Slow sync process", slog.String("cron", freq))
			}
			if err := fn(); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}
