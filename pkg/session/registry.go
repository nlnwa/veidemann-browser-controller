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
	"fmt"
	"github.com/nlnwa/veidemann-browser-controller/pkg/metrics"
	log "github.com/sirupsen/logrus"
	"sync"
	"time"
)

type SessionRegistry struct {
	sessions []*Session
	ctx      context.Context
	mu       sync.Mutex
	opts     []SessionOption
	wg       *sync.WaitGroup
}

func NewSessionRegistry(ctx context.Context, maxSessions int, opts ...SessionOption) (sr *SessionRegistry) {
	sr = &SessionRegistry{
		sessions: make([]*Session, maxSessions),
		ctx:      ctx,
		opts:     opts,
		wg:       &sync.WaitGroup{},
	}
	metrics.BrowserSessions.Set(float64(maxSessions))
	return
}

func (sr *SessionRegistry) GetNextAvailable() (sess *Session, err error) {
	for {
		select {
		case <-sr.ctx.Done():
			return nil, fmt.Errorf("cancelled")
		default:
			for i := 1; i < len(sr.sessions); i++ {
				sr.mu.Lock()
				if sr.sessions[i] == nil {
					sr.wg.Add(1)
					sess, err = New(i, sr.opts...)
					sr.sessions[i] = sess
					metrics.ActiveBrowserSessions.Set(float64(sr.CurrentSessions()))
					sr.mu.Unlock()
					return
				} else {
					sr.mu.Unlock()
				}
			}
		}
	}
}

func (sr *SessionRegistry) NewDirectSession(uri, crawlExecutionId, jobExecutionId string) (*Session, error) {
	return newDirectSession(uri, crawlExecutionId, jobExecutionId, sr.opts...)
}

func (sr *SessionRegistry) Get(sessId int) *Session {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	s := sr.sessions[sessId]
	return s
}

func (sr *SessionRegistry) Release(sess *Session) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	sr.sessions[sess.Id] = nil
	metrics.ActiveBrowserSessions.Set(float64(sr.CurrentSessions()))
	sr.wg.Done()
}

func (sr *SessionRegistry) MaxSessions() int {
	return len(sr.sessions)
}

func (sr *SessionRegistry) CurrentSessions() int {
	c := 0
	for _, s := range sr.sessions {
		if s != nil {
			c++
		}
	}
	return c
}

func (sr *SessionRegistry) CloseWait(timeout time.Duration) {
	c := make(chan struct{})
	go func() {
		defer close(c)
		sr.wg.Wait()
	}()
	select {
	case <-c:
		log.Infof("All sessions finished")
		metrics.ActiveBrowserSessions.Set(float64(sr.CurrentSessions()))
		metrics.BrowserSessions.Set(0)
		return
	case <-time.After(timeout):
		log.Infof("Timed out waiting for %d sessions to finish.", sr.CurrentSessions())
		metrics.ActiveBrowserSessions.Set(0)
		metrics.BrowserSessions.Set(0)
		close(c)
		return
	}
}
