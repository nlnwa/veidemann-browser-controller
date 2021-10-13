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
	"sync/atomic"
)

type WaitGroup struct {
	ctx     context.Context
	counter int32
	done    chan interface{}
	cancel  chan interface{}
	state   int32
}

func NewWaitGroup(ctx context.Context) *WaitGroup {
	return &WaitGroup{
		ctx:     ctx,
		counter: 0,
		done:    make(chan interface{}),
		cancel:  make(chan interface{}),
	}
}

func (wg *WaitGroup) Add(delta int) {
	n := atomic.AddInt32(&wg.counter, int32(delta))
	if n == 0 && atomic.CompareAndSwapInt32(&wg.state, 1, 2) {
		close(wg.done)
	}
}

func (wg *WaitGroup) Done() {
	wg.Add(-1)
}

func (wg *WaitGroup) Wait() (err error) {
	atomic.CompareAndSwapInt32(&wg.state, 0, 1)
	if atomic.LoadInt32(&wg.counter) == 0 {
		return nil
	}
	select {
	case <-wg.ctx.Done():
		return ExceededMaxTime
	case <-wg.done:
		return nil
	case <-wg.cancel:
		return Cancelled
	}
}

func (wg *WaitGroup) Cancel() {
	close(wg.cancel)
}
