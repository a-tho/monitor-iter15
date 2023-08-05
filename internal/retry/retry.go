package retry

import (
	"context"
	"errors"
	"time"
)

type retriableError struct {
	err error
}

type retriableFunc func(context.Context) error

func RetriableError(err error) *retriableError {
	return &retriableError{err: err}
}

func (e retriableError) Error() string {
	if e.err == nil {
		return "retriable: <nil>"
	}
	return "retriable: " + e.err.Error()
}

func (e retriableError) Unwrap() error {
	return e.err
}

// Do performs up to three retries with 1, 3 and 5 seconds. f is expected to
// wrap retriable errors with RetriableError method to let Do know that a retry
// should be performed.
func Do(ctx context.Context, f retriableFunc) error {
	intervals := []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second}

	var err error
	for _, interval := range intervals {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err = f(ctx)
		if err == nil {
			return nil
		}

		var rerr retriableError
		if !errors.As(err, &rerr) {
			return err
		}

		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return errors.Unwrap(err)
}
