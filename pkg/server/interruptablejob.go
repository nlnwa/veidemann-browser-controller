package server

import (
	"context"
	"github.com/nlnwa/veidemann-api-go/browsercontroller/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

func Run(ctx context.Context, fn func() (*browsercontroller.DoRequest, error)) (*browsercontroller.DoRequest, error) {
	var result *browsercontroller.DoRequest
	var err error
	done := make(chan interface{})

	go func() {
		result, err = fn()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return result, ctx.Err()
	case <-done:
		return result, err
	}
}

// DoWithTimeout runs f and returns its error.  If the deadline d elapses first,
// it returns a grpc DeadlineExceeded error instead.
func DoWithTimeout(f func() error, d time.Duration) error {
	errChan := make(chan error, 1)
	go func() {
		errChan <- f()
		close(errChan)
	}()
	t := time.NewTimer(d)
	select {
	case <-t.C:
		return status.Errorf(codes.DeadlineExceeded, "too slow")
	case err := <-errChan:
		if !t.Stop() {
			<-t.C
		}
		return err
	}
}

// DoWithContext runs f and returns its error.  If the context is cancelled or
// times out first, it returns the context's error instead.
func DoWithContext(ctx context.Context, f func() error) error {
	errChan := make(chan error, 1)
	go func() {
		errChan <- f()
		close(errChan)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}
