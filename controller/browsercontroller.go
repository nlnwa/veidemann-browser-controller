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
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

type BrowserController struct {
	opts browserControllerOptions
	done chan error
}

func New(opts ...BrowserControllerOption) *BrowserController {
	bc := &BrowserController{
		opts: defaultBrowserControllerOptions(),
		done: make(chan error, 1),
	}
	for _, opt := range opts {
		opt.apply(&bc.opts)
	}
	return bc
}

func (bc *BrowserController) Run() error {
	sessions := session.NewRegistry(
		bc.opts.maxSessions,
		bc.opts.sessionOpts...,
	)

	// ctx is used to signal (via cancellation) if the api server stops unexpectedly
	ctx, cancel := context.WithCancel(context.Background())
	apiServer := server.NewApiServer(bc.opts.listenInterface, bc.opts.listenPort, sessions, bc.opts.robotsEvaluator, bc.opts.logWriter)
	go func() {
		if err := apiServer.Start(); err != nil {
			bc.done <- fmt.Errorf("API server stopped: %w", err)
			cancel()
		}
	}()
	defer apiServer.Close()

	defer sessions.CloseWait(bc.opts.closeTimeout)

	// give api server time to start
	time.Sleep(time.Millisecond)

	log.Info().Msg("Browser Controller started")

	backoff := make(chan time.Time, 1)
	for {
		select {
		case err := <-bc.done:
			return err
		case <-backoff:
			d := 10 * time.Second
			log.Debug().Dur("durationMs", d).Msg("Next page not found, backing off..")
			time.Sleep(d)
		default:
			// get session
			sess, err := sessions.GetNextAvailable(ctx)
			if err != nil {
				switch err {
				case context.Canceled:
					continue
				default:
					return fmt.Errorf("failed to get next session: %w", err)
				}
			}

			// get a page to fetch
			phs, err := bc.opts.frontier.GetNextPage(ctx)
			if err != nil {
				sessions.Release(sess)
				if st, ok := status.FromError(err); ok {
					switch st.Code() {
					case codes.NotFound:
						backoff <- time.Now()
						continue
					}
				}
				return fmt.Errorf("failed to get next page: %w", err)
			}

			// fetch and report
			go func() {
				sess := sess
				phs := phs
				defer sessions.Release(sess)

				result, fetchErr := sess.Fetch(ctx, phs)
				if err := bc.opts.frontier.PageCompleted(phs, result, fetchErr); err != nil {
					log.Error().Err(err).Msg("Error reporting page completed")
				}
			}()
		}
	}
}

func (bc *BrowserController) Shutdown() {
	log.Info().Msg("Shutting down Browser Controller...")
	bc.done <- nil
}
