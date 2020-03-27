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
	frontierV1 "github.com/nlnwa/veidemann-api-go/frontier/v1"
	"github.com/nlnwa/veidemann-browser-controller/pkg/errors"
	"github.com/nlnwa/veidemann-browser-controller/pkg/metrics"
	"github.com/nlnwa/veidemann-browser-controller/pkg/session"
	log "github.com/sirupsen/logrus"
	"io"
	"strconv"
)

func (bc *BrowserController) Harvest(ctx context.Context, sess *session.Session) {
	stream, err := bc.opts.frontierConn.Client().GetNextPage(ctx)
	if err != nil {
		log.Fatalf("Failed to open Frontier session: %v", err)
	}
	waitc := make(chan struct{}) // Closed when session is completed/failed
	waitn := make(chan struct{}) // Closed when initial response from Frontier is received
	var harvestSpec *frontierV1.PageHarvestSpec
	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				// read done.
				close(waitc)
				return
			}
			if err != nil {
				log.Errorf("Failed to receive Frontier response: %v", err)
				close(waitc)
				return
			}
			harvestSpec = in
			close(waitn)
		}
	}()
	if err := stream.Send(&frontierV1.PageHarvest{Msg: &frontierV1.PageHarvest_RequestNextPage{RequestNextPage: true}}); err != nil {
		log.Errorf("Failed to send request for new page to Frontier: %v", err)
		_ = stream.CloseSend()
		bc.sessions.Release(sess)
		return
	}
	select {
	case <-bc.ctx.Done():
		log.Debugf("Session #%d is cancelled", sess.Id)
		_ = stream.CloseSend()
		bc.sessions.Release(sess)
		return
	case <-waitn:
	}

	log.WithField("uri", harvestSpec.QueuedUri.Uri).Tracef("Starting fetch")
	metrics.PagesTotal.Inc()
	renderResult, err := sess.Fetch(harvestSpec.QueuedUri, harvestSpec.CrawlConfig)
	if err != nil {
		renderResult = &session.RenderResult{
			Error: errors.CommonsError(err),
		}
		log.Errorf("Failed to Fetch: %v", err)
	}

	if renderResult.PageFetchTimeMs > 0 {
		metrics.PageFetchSeconds.Observe(float64(renderResult.PageFetchTimeMs / 1000))
	}

	if renderResult.Error != nil {
		if err := stream.Send(&frontierV1.PageHarvest{Msg: &frontierV1.PageHarvest_Error{Error: renderResult.Error}}); err != nil {
			log.Errorf("Failed to send error response to Frontier: %v", err)
			_ = stream.CloseSend()
			bc.sessions.Release(sess)
			metrics.PagesFailedTotal.WithLabelValues(strconv.Itoa(int(renderResult.Error.Code))).Inc()
			return
		}
	} else {
		if err := stream.Send(&frontierV1.PageHarvest{
			Msg: &frontierV1.PageHarvest_Metrics_{
				Metrics: &frontierV1.PageHarvest_Metrics{
					UriCount:        renderResult.UriCount,
					BytesDownloaded: renderResult.BytesDownloaded,
				},
			},
		}); err != nil {
			log.Errorf("Failed to send metrics to Frontier: %v", err)
			_ = stream.CloseSend()
			bc.sessions.Release(sess)
			return
		}

		for _, outlink := range renderResult.Outlinks {
			if err := stream.Send(&frontierV1.PageHarvest{Msg: &frontierV1.PageHarvest_Outlink{Outlink: outlink}}); err != nil {
				log.Errorf("Failed to send outlink to Frontier: %v", err)
			}
		}
	}

	_ = stream.CloseSend()
	<-waitc
	bc.sessions.Release(sess)
}
