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
	"github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	"time"
)

type BrowserController struct {
	opts   browserControllerOptions
	cancel func()
}

func New(opts ...BrowserControllerOption) *BrowserController {
	bc := &BrowserController{
		opts: defaultBrowserControllerOptions(),
	}
	for _, opt := range opts {
		opt.apply(&bc.opts)
	}
	return bc
}

func (bc *BrowserController) Run() error {
	var ctx context.Context
	ctx, bc.cancel = context.WithCancel(context.Background())

	sessions := session.NewRegistry(
		bc.opts.maxSessions,
		bc.opts.sessionOpts...,
	)

	apiServer := server.NewApiServer(bc.opts.listenInterface, bc.opts.listenPort, sessions, bc.opts.robotsEvaluator)
	go func() {
		err := apiServer.Start()
		if err != nil {
			log.WithError(err).Error("Api server failed")
			bc.cancel()
		}
	}()

	defer apiServer.Close()
	defer sessions.CloseWait(bc.opts.closeTimeout)

	// give apiServer a chance to start
	time.Sleep(time.Millisecond)

	log.Infof("Browser Controller started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			sess, err := sessions.GetNextAvailable(ctx)
			if err != nil {
				if err == context.Canceled {
					break
				}
				return fmt.Errorf("failed to get session: %w", err)
			}
			go func() {
				span := opentracing.StartSpan("harvest")
				defer span.Finish()
				ctx := opentracing.ContextWithSpan(context.Background(), span)
				err := bc.opts.harvester.Harvest(ctx, sess.Fetch)
				if err != nil {
					if err == context.Canceled {
						log.Debugf("Harvest session #%v canceled", sess.Id)
					} else {
						log.Warnf("Harvest completed with error: %v", err)
					}
					time.Sleep(time.Second)
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
