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
	"github.com/nlnwa/veidemann-browser-controller/pkg/harvester"
	"github.com/nlnwa/veidemann-browser-controller/pkg/session"
	log "github.com/sirupsen/logrus"
	"time"
)

type BrowserController struct {
	sessions  *session.Registry
	harvester harvester.Harvester
}

func New(sessions *session.Registry, harvester harvester.Harvester) *BrowserController {
	return &BrowserController{
		sessions:  sessions,
		harvester: harvester,
	}
}

func (bc *BrowserController) Run(ctx context.Context) error {
	for {
		sess, err := bc.sessions.GetNextAvailable(ctx)
		if err != nil {
			return fmt.Errorf("failed to get session: %w", err)
		}
		go func() {
			err := bc.harvester.Harvest(ctx, sess.Fetch)
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
