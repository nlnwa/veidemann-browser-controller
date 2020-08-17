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

package harvester

import (
	"context"
	"fmt"
	commonsV1 "github.com/nlnwa/veidemann-api-go/commons/v1"
	configV1 "github.com/nlnwa/veidemann-api-go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api-go/frontier/v1"
	"github.com/nlnwa/veidemann-browser-controller/pkg/errors"
	"github.com/nlnwa/veidemann-browser-controller/pkg/metrics"
	"github.com/nlnwa/veidemann-browser-controller/pkg/serviceconnections"
	log "github.com/sirupsen/logrus"
	"io"
	"strconv"
	"sync"
)

type RenderResult struct {
	BytesDownloaded int64
	UriCount        int32
	Outlinks        []*frontierV1.QueuedUri
	Error           *commonsV1.Error
	PageFetchTimeMs int64
}

type FetchFunc func(*frontierV1.QueuedUri, *configV1.ConfigObject) (*RenderResult, error)

type Harvester interface {
	Harvest(context.Context, FetchFunc) error
}

type harvester struct {
	conn *serviceconnections.FrontierConn
}

func New(conn *serviceconnections.FrontierConn) Harvester {
	return &harvester{conn}
}

func (f *harvester) Harvest(ctx context.Context, fetch FetchFunc) error {
	harvestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := f.conn.Client().GetNextPage(harvestCtx)
	if err != nil {
		return fmt.Errorf("failed to get the frontier GetNextPage client: %w", err)
	}

	errc := make(chan error)
	waitn := make(chan struct{}) // Closed when initial response from Frontier is received
	var harvestSpec *frontierV1.PageHarvestSpec
	go func() {
		once := new(sync.Once)
		for {
			var err error
			harvestSpec, err = stream.Recv()
			if err == io.EOF {
				// stream completed
				close(errc)
				return
			}
			if err != nil {
				errc <- fmt.Errorf("failed to receive response from frontier: %w", err)
				return
			}
			once.Do(func() {
				close(waitn)
			})
		}
	}()
	go func() {
		err := stream.Send(&frontierV1.PageHarvest{Msg: &frontierV1.PageHarvest_RequestNextPage{RequestNextPage: true}})
		// If the error is not io.EOF it is a client side error, else the status is read by stream.Recv above.
		if err != nil && err != io.EOF {
			errc <- fmt.Errorf("failed to send request for new page to frontier: %w", err)
		}
	}()
	select {
	case <-ctx.Done():
		// ctx cancelled while requesting next page
		return ctx.Err()
	case err := <-errc:
		return err
	case <-waitn:
		// initial request received
	}

	log.WithField("uri", harvestSpec.QueuedUri.Uri).Tracef("Starting fetch")
	metrics.PagesTotal.Inc()
	renderResult, err := fetch(harvestSpec.QueuedUri, harvestSpec.CrawlConfig)
	if err != nil {
		log.Errorf("Failed to fetch: %v", err)
		renderResult = &RenderResult{
			Error: errors.CommonsError(err),
		}
	} else if renderResult.PageFetchTimeMs > 0 {
		metrics.PageFetchSeconds.Observe(float64(renderResult.PageFetchTimeMs / 1000))
	}

	if renderResult.Error != nil {
		err := stream.Send(&frontierV1.PageHarvest{Msg: &frontierV1.PageHarvest_Error{Error: renderResult.Error}})
		if err != nil {
			metrics.PagesFailedTotal.WithLabelValues(strconv.Itoa(int(renderResult.Error.Code))).Inc()
			return fmt.Errorf("failed to send error response to Frontier: %w", err)
		}
	} else {
		err := stream.Send(&frontierV1.PageHarvest{
			Msg: &frontierV1.PageHarvest_Metrics_{
				Metrics: &frontierV1.PageHarvest_Metrics{
					UriCount:        renderResult.UriCount,
					BytesDownloaded: renderResult.BytesDownloaded,
				},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to send metrics to Frontier: %w", err)
		}

		for _, outlink := range renderResult.Outlinks {
			if err := stream.Send(&frontierV1.PageHarvest{Msg: &frontierV1.PageHarvest_Outlink{Outlink: outlink}}); err != nil {
				return fmt.Errorf("failed to send outlink to Frontier: %w", err)
			}
		}
	}

	if err := stream.CloseSend(); err != nil {
		return fmt.Errorf("failed to close send: %w", err)
	}
	return <-errc
}