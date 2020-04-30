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
	"github.com/nlnwa/veidemann-browser-controller/pkg/server"
	"github.com/nlnwa/veidemann-browser-controller/pkg/session"
	log "github.com/sirupsen/logrus"
)

type BrowserController struct {
	ctx         context.Context
	cancel      context.CancelFunc
	opts        browserControllerOptions
	sessions    *session.SessionRegistry
	waitRunning chan struct{}
}

func New(opts ...BrowserControllerOption) *BrowserController {
	bc := &BrowserController{
		opts: defaultBrowserControllerOptions(),
	}
	for _, opt := range opts {
		opt.apply(&bc.opts)
	}
	bc.waitRunning = make(chan struct{})
	return bc
}

func (bc *BrowserController) Start() {
	log.Infof("Starting Browser Controller ...")

	bc.ctx, bc.cancel = context.WithCancel(context.Background())

	bc.opts.sessionOpts = append(bc.opts.sessionOpts,
		session.WithIsAllowedByRobotsTxtFunc(bc.funcIsAllowed),
		session.WithWriteScreenshotFunc(bc.writeScreenshot))
	bc.sessions = session.NewSessionRegistry(
		bc.ctx,
		bc.opts.maxSessions,
		bc.opts.sessionOpts...,
	)

	apiServer := server.NewApiServer(bc.opts.listenInterface, bc.opts.listenPort, bc.sessions)
	apiServer.Start()

	waitc := make(chan struct{})

	go func() {
		for {
			sess, err := bc.sessions.GetNextAvailable()
			if err != nil {
				close(waitc)
				return
			}
			go bc.Harvest(bc.ctx, sess)
		}
	}()

	log.Infof("Browser Controller started")

	<-waitc
	log.Debugf("Stop requested. Waiting for %v remaining sessions", bc.sessions.CurrentSessions())
	bc.sessions.CloseWait(bc.opts.closeTimeout)
	apiServer.Close()
	close(bc.waitRunning)
}

func (bc *BrowserController) Stop() {
	log.Infof("Stopping Browser Controller ...")
	bc.cancel()
	<-bc.waitRunning
	log.Infof("Browser Controller stopped")
}
