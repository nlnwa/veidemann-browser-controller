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

package syncx

import (
	"context"
	"testing"
	"time"
)

func Test_WaitGroup_Wait(t *testing.T) {
	wg := NewWaitGroup(context.Background())

	err := wg.Wait()
	if err != nil {
		t.Errorf("No error expected. Got: %v", err)
	}
}

func Test_WaitGroup_Done(t *testing.T) {
	wg := NewWaitGroup(context.Background())

	wg.Add(1)
	go func() {
		time.AfterFunc(time.Millisecond*100, func() {
			wg.Done()
		})
	}()

	err := wg.Wait()
	if err != nil {
		t.Errorf("No error expected. Got: %v", err)
	}
}

func Test_WaitGroup_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	wg := NewWaitGroup(ctx)

	wg.Add(1)
	go func() {
		time.AfterFunc(time.Millisecond*100, func() {
			cancel()
		})
	}()

	err := wg.Wait()
	if err != ExceededMaxTime {
		t.Errorf("Wanted err: %v. Got: %v", ExceededMaxTime, err)
	}

	wg = NewWaitGroup(context.Background())

	wg.Add(1)
	go func() {
		time.AfterFunc(time.Millisecond*100, func() {
			wg.Cancel()
		})
	}()

	err = wg.Wait()
	if err != Cancelled {
		t.Errorf("Wanted err: %v. Got: %v", Cancelled, err)
	}
}

func Test_WaitGroup_After_Done(t *testing.T) {
	wg := NewWaitGroup(context.Background())

	wg.Add(1)
	go func() {
		time.AfterFunc(time.Millisecond*100, func() {
			wg.Done()
		})
	}()
	err := wg.Wait()
	if err != nil {
		t.Errorf("No error expected. Got: %v", err)
	}

	// Test that nothing happens if a WaitGroup which is done, is manipulated
	wg.Add(1)
	wg.Done()
	err = wg.Wait()
	if err != nil {
		t.Errorf("No error expected. Got: %v", err)
	}
}
