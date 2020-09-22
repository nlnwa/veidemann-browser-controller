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

package controller

import (
	"context"
	"fmt"
	"github.com/nlnwa/veidemann-browser-controller/pkg/server"
	"github.com/nlnwa/veidemann-browser-controller/pkg/session"
	log "github.com/sirupsen/logrus"
	"time"
)

type BrowserController struct {
	opts     browserControllerOptions
	sessions *session.Registry
	stopping chan bool
}

func New(opts ...BrowserControllerOption) *BrowserController {
	bc := &BrowserController{
		opts:     defaultBrowserControllerOptions(),
		stopping: make(chan bool, 1),
	}
	for _, opt := range opts {
		opt.apply(&bc.opts)
	}
	return bc
}

func (bc *BrowserController) Run(ctx context.Context) error {
	bc.sessions = session.NewRegistry(
		bc.opts.maxSessions,
		bc.opts.sessionOpts...,
	)

	apiServer := server.NewApiServer(bc.opts.listenInterface, bc.opts.listenPort, bc.sessions, bc.opts.robotsEvaluator)
	go func() error { return apiServer.Start() }()

	defer apiServer.Close()
	defer bc.sessions.CloseWait(bc.opts.closeTimeout)

	// give apiServer a chance to start
	<-time.After(time.Millisecond)

	log.Infof("Browser Controller started")

	for {
		select {
		case <-ctx.Done():
			bc.Stop()
			return ctx.Err()
		case <-bc.stopping:
			return fmt.Errorf("browser controller requested to stop")
		default:
			sess, err := bc.sessions.GetNextAvailable(ctx)
			if err != nil {
				return fmt.Errorf("failed to get session: %w", err)
			}
			go func() {
				err := bc.opts.harvester.Harvest(ctx, sess.Fetch)
				if err != nil {
					log.Warnf("Harvest completed with error: %v", err)
					<-time.After(time.Second)
				}
				if sess != nil {
					bc.sessions.Release(sess)
				}
			}()
		}
	}
}

func (bc *BrowserController) Stop() {
	log.Infof("Stopping Browser Controller ...")
	close(bc.stopping)
}
