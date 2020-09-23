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
	shutdown chan error
}

func New(opts ...BrowserControllerOption) *BrowserController {
	bc := &BrowserController{
		opts:     defaultBrowserControllerOptions(),
		shutdown: make(chan error),
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
	go func() {
		bc.shutdown <- apiServer.Start()
	}()

	defer apiServer.Close()
	defer bc.sessions.CloseWait(bc.opts.closeTimeout)

	// give apiServer a chance to start
	<-time.After(time.Millisecond)

	bcCtx, bcCancel := context.WithCancel(ctx)
	go func() {
		select {
		case err := <-bc.shutdown:
			if err != nil {
				log.WithError(err).Errorf("api server failed")
			}
			bcCancel()
		case <-ctx.Done():
			log.Infof("Browser Controller canceled")
			bc.shutdown <- nil
		}
	}()

	log.Infof("Browser Controller started")

	for {
		select {
		case <-bcCtx.Done():
			return ctx.Err()
		default:
			sess, err := bc.sessions.GetNextAvailable(bcCtx)
			if err != nil {
				if err == context.Canceled {
					break
				}
				return fmt.Errorf("failed to get session: %w", err)
			}
			go func() {
				err := bc.opts.harvester.Harvest(bcCtx, sess.Fetch)
				if err != nil {
					if err == context.Canceled {
						log.Debugf("Harvest session #%v canceled", sess.Id)
					} else {
						log.Warnf("Harvest completed with error: %v", err)
					}
					<-time.After(time.Second)
				}
				if sess != nil {
					bc.sessions.Release(sess)
				}
			}()
		}
	}
}

func (bc *BrowserController) Shutdown() {
	log.Infof("Shutting down Browser Controller...")
	bc.shutdown <- nil
}
