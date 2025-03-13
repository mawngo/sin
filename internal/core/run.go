package core

import (
	"context"
	"errors"
	"time"
)

// Run execute the function with given frequency.
// Run stop if the function returns an error.
func Run(ctx context.Context, feq string, fn func() error) error {
	if feq == "" {
		return fn()
	}

	if dur, err := time.ParseDuration(feq); err == nil {
		return runInterval(ctx, dur, fn)
	}

	// TODO: support cron
	return errors.New("unsupported frequency: " + feq)
}

func runInterval(ctx context.Context, dur time.Duration, fn func() error) error {
	timer := time.NewTimer(dur)
	if err := fn(); err != nil {
		return err
	}
	for {
		select {
		case <-timer.C:
			timer = time.NewTimer(dur)
			if err := fn(); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}
