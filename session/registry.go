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

package session

import (
	"context"
	"github.com/nlnwa/veidemann-browser-controller/metrics"
	log "github.com/sirupsen/logrus"
	"sync"
	"time"
)

type Registry struct {
	sessions        []*Session
	pool            chan int
	mu              sync.Mutex
	opts            []Option
	wg              *sync.WaitGroup
	createdSessions int32
}

func NewRegistry(maxSessions int, opts ...Option) (sr *Registry) {
	sr = &Registry{
		sessions: make([]*Session, maxSessions),
		pool:     make(chan int, maxSessions-1),
		opts:     opts,
		wg:       &sync.WaitGroup{},
	}
	for i := 1; i < maxSessions; i++ {
		sr.pool <- i
	}
	metrics.BrowserSessions.Set(float64(maxSessions))
	return
}

// GetNextAvailable returns next session from the pool.
func (sr *Registry) GetNextAvailable(ctx context.Context) (*Session, error) {
	var i int
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case i = <-sr.pool:
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sess, err := New(i, sr.opts...)
	if err != nil {
		return nil, err
	}
	sr.wg.Add(1)
	sr.sessions[i] = sess
	return sess, nil
}

func (sr *Registry) NewDirectSession(uri, crawlExecutionId, jobExecutionId string) (*Session, error) {
	return newDirectSession(uri, crawlExecutionId, jobExecutionId, sr.opts...)
}

func (sr *Registry) Get(sessId int) *Session {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	s := sr.sessions[sessId]
	return s
}

func (sr *Registry) Release(sess *Session) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	sr.sessions[sess.Id] = nil
	sr.wg.Done()
	sr.pool <- sess.Id
}

func (sr *Registry) MaxSessions() int {
	return len(sr.sessions)
}

func (sr *Registry) CurrentSessions() int {
	c := 0
	for _, s := range sr.sessions {
		if s != nil {
			c++
		}
	}
	return c
}

func (sr *Registry) CloseWait(timeout time.Duration) {
	c := make(chan struct{})
	go func() {
		sr.wg.Wait()
		close(c)
	}()
	log.Debugf("Waiting for %v remaining sessions", sr.CurrentSessions())
	select {
	case <-c:
		log.Infof("All sessions finished")
		metrics.ActiveBrowserSessions.Set(0)
		metrics.BrowserSessions.Set(0)
	case <-time.After(timeout):
		log.Infof("Timed out waiting for %d sessions to finish.", sr.CurrentSessions())
		metrics.ActiveBrowserSessions.Set(0)
		metrics.BrowserSessions.Set(0)
	}
}
