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
	"github.com/nlnwa/veidemann-browser-controller/server"
	"github.com/nlnwa/veidemann-browser-controller/session"
	log "github.com/sirupsen/logrus"
	"time"
)

type BrowserController struct {
	opts   browserControllerOptions
	ctx    context.Context
	cancel func()
}

func New(opts ...BrowserControllerOption) *BrowserController {
	ctx, cancel := context.WithCancel(context.Background())

	bc := &BrowserController{
		opts:   defaultBrowserControllerOptions(),
		cancel: cancel,
		ctx:    ctx,
	}
	for _, opt := range opts {
		opt.apply(&bc.opts)
	}
	return bc
}

func (bc *BrowserController) Run() (err error) {
	sessions := session.NewRegistry(
		bc.opts.maxSessions,
		bc.opts.sessionOpts...,
	)

	apiServer := server.NewApiServer(bc.opts.listenInterface, bc.opts.listenPort, sessions, bc.opts.robotsEvaluator, bc.opts.logWriter)
	go func() {
		err = apiServer.Start()
		if err != nil {
			err = fmt.Errorf("API server failed: %w", err)
			bc.cancel()
		}
	}()
	defer apiServer.Close()

	defer sessions.CloseWait(bc.opts.closeTimeout)

	// give api server time to start
	time.Sleep(time.Millisecond)

	log.Infof("Browser Controller started")
	for {
		select {
		case <-bc.ctx.Done():
			return
		default:
			sess, err := sessions.GetNextAvailable(bc.ctx)
			if err != nil {
				return fmt.Errorf("failed to get session: %w", err)
			}
			go func() {
				if err := bc.opts.harvester.Harvest(bc.ctx, sess.Fetch); err != nil {
					log.WithError(err).WithField("session", sess.Id).Warning("Harvest error")
					time.Sleep(time.Second) // delay release of session after error
				}
				if sess != nil {
					sessions.Release(sess)
				}
			}()
		}
	}
}

func (bc *BrowserController) Shutdown() {
	log.Infof("Shutting down Browser Controller...")
	bc.cancel()
}
