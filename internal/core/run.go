package core

import (
	"context"
	"github.com/flc1125/go-cron/v4"
	"github.com/mawngo/go-errors"
	"github.com/pterm/pterm"
	"log/slog"
	"strings"
	"time"
)

// Run execute the function with given frequency without overlapping.
// Run stop if the function returns an error.
func Run(ctx context.Context, freq string, fn func() error) error {
	if freq == "" {
		return fn()
	}

	immediate := false
	if strings.HasSuffix(freq, "!") {
		immediate = true
		freq = strings.TrimSuffix(freq, "!")
	}

	if dur, err := time.ParseDuration(freq); err == nil {
		return runInterval(ctx, dur, immediate, fn)
	}

	return runCron(ctx, freq, immediate, fn)
}

func runInterval(ctx context.Context, dur time.Duration, immediate bool, fn func() error) error {
	timer := time.NewTimer(dur)
	startWait := time.Now()

	if immediate {
		if err := fn(); err != nil {
			return err
		}
	}

	for {
		select {
		case <-timer.C:
			if time.Since(startWait) < 10*time.Second {
				pterm.Warning.Println("Sync can't keep up with the frequency")
				pterm.Warning.Println("Sync job take too long or the frequency is too fast")
				slog.Warn("Slow sync process", slog.String("freq", dur.String()))
			}
			timer = time.NewTimer(dur)
			startWait = time.Now()
			if err := fn(); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func runCron(ctx context.Context, freq string, immediate bool, fn func() error) error {
	c := cron.New(
		cron.WithContext(ctx),
		cron.WithLogger(cron.DiscardLogger),
	)
	defer c.Stop()

	// Queue the job, so if the job can't keep up with the frequency,
	// it can still be executed (but only once).
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
		return errors.Wrapf(err, "invalid cron expression [%s]", freq)
	}
	c.Start()
	if immediate {
		select {
		case jobs <- struct{}{}:
		case <-ctx.Done():
		default:
		}
	}
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
