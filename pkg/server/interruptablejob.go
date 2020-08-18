/*
 * Copyright 2020 National Library of Norway.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package server

import (
	"context"
	browsercontrollerV1 "github.com/nlnwa/veidemann-api-go/browsercontroller/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

func Send(fn func(*browsercontrollerV1.DoReply) error, reply *browsercontrollerV1.DoReply) error {
	return DoWithTimeout(func() error { return fn(reply) }, 5*time.Second)
}

func Recv(ctx context.Context, fn func() (*browsercontrollerV1.DoRequest, error)) (*browsercontrollerV1.DoRequest, error) {
	var result *browsercontrollerV1.DoRequest
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
