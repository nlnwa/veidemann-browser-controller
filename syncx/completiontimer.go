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
	"errors"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

var IdleTimeout = errors.New("idle timeout")
var ExceededMaxTime = errors.New("exceeded max time")
var Cancelled = errors.New("cancelled")

type CompletionTimer struct {
	maxIdleTime     time.Duration
	maxTotalTime    time.Duration
	check           func() bool
	lastCheckResult bool
	waitTimer       *time.Timer
	idleTimer       *time.Timer
	idleTimeout     time.Time
	doneChan        chan interface{}
	started         bool
	done            int32 // 0: not done, 1: success, 2: cancel
	notifyCount     int32
}

func NewCompletionTimer(maxIdleTime, maxTotalTime time.Duration, check func() bool) *CompletionTimer {
	log.Trace().
		Dur("maxIdleTime", maxIdleTime).
		Dur("maxTotalTime", maxTotalTime).
		Msg("Completion timer")
	if check == nil {
		check = func() bool {
			return false
		}
	}
	return &CompletionTimer{
		maxIdleTime:  maxIdleTime,
		maxTotalTime: maxTotalTime,
		check:        check,
		doneChan:     make(chan interface{}),
	}
}

func (t *CompletionTimer) Notify() {
	atomic.AddInt32(&t.notifyCount, 1)
	t.idleTimeout = time.Now().Add(t.maxIdleTime)

	if !t.started {
		return
	}

	t.lastCheckResult = t.check()
	if t.lastCheckResult {
		if atomic.CompareAndSwapInt32(&t.done, 0, 1) {
			close(t.doneChan)
			return
		}
	}
}

func (t *CompletionTimer) WaitForCompletion() (err error) {
	t.lastCheckResult = t.check()
	if t.lastCheckResult {
		return
	}
	t.waitTimer = time.NewTimer(t.maxTotalTime)
	t.idleTimeout = time.Now().Add(t.maxIdleTime)
	t.idleTimer = time.NewTimer(t.maxIdleTime)
	t.started = true

	for {
		select {
		case <-t.waitTimer.C:
			t.lastCheckResult = t.check()
			if t.lastCheckResult {
				return
			} else {
				return ExceededMaxTime
			}
		case <-t.idleTimer.C:
			t.lastCheckResult = t.check()
			if t.lastCheckResult {
				return
			} else {
				if time.Now().After(t.idleTimeout) {
					return IdleTimeout
				} else {
					t.idleTimer = time.NewTimer(time.Until(t.idleTimeout))
				}
			}
		case <-t.doneChan:
			if atomic.LoadInt32(&t.done) == 1 {
				return
			} else {
				return Cancelled
			}
		}
	}
}

func (t *CompletionTimer) Cancel() {
	if atomic.CompareAndSwapInt32(&t.done, 0, 2) {
		close(t.doneChan)
	}
}

// Reset lets the completion timer do a new WaitForCompletion if the previous returned an IdleTimeout
// Reset returns the number of notifications received since last reset
func (t *CompletionTimer) Reset() int32 {
	atomic.CompareAndSwapInt32(&t.done, 1, 0)
	t.started = false
	return atomic.SwapInt32(&t.notifyCount, 0)
}
