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
	commonsV1 "github.com/nlnwa/veidemann-api/go/commons/v1"
	configV1 "github.com/nlnwa/veidemann-api/go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api/go/frontier/v1"
	"github.com/nlnwa/veidemann-browser-controller/errors"
	"github.com/nlnwa/veidemann-browser-controller/metrics"
	"github.com/nlnwa/veidemann-browser-controller/serviceconnections"
	"github.com/opentracing/opentracing-go"
	tracelog "github.com/opentracing/opentracing-go/log"
	"github.com/rs/zerolog/log"
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

type FetchFunc func(context.Context, *frontierV1.QueuedUri, *configV1.ConfigObject) (*RenderResult, error)

type Harvester interface {
	serviceconnections.Connection
	Harvest(context.Context, FetchFunc) error
}

type harvester struct {
	*serviceconnections.ClientConn
	frontierV1.FrontierClient
}

func New(opts ...serviceconnections.ConnectionOption) Harvester {
	return &harvester{
		ClientConn: serviceconnections.NewClientConn("Frontier", opts...),
	}
}

func (h *harvester) Connect() error {
	if err := h.ClientConn.Connect(); err != nil {
		return err
	} else {
		h.FrontierClient = frontierV1.NewFrontierClient(h.ClientConn.Connection())
		return nil
	}
}

func (h *harvester) Harvest(ctx context.Context, fetch FetchFunc) error {
	span := opentracing.StartSpan("harvest").SetTag("component", "harvester")
	defer span.Finish()
	spanCtx := opentracing.ContextWithSpan(context.Background(), span)

	harvestCtx, cancel := context.WithCancel(spanCtx)
	defer cancel()

	stream, err := h.FrontierClient.GetNextPage(harvestCtx)
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
				errc <- err
				return
			}
			once.Do(func() {
				close(waitn)
			})
		}
	}()

	err = stream.Send(&frontierV1.PageHarvest{Msg: &frontierV1.PageHarvest_RequestNextPage{RequestNextPage: true}})
	if err != nil {
		if err == io.EOF {
			err = <-errc
		}
		return fmt.Errorf("failed to send request for new page to frontier: %w", err)
	}

	select {
	case <-ctx.Done():
		// ctx cancelled while waiting for harvest spec
		_ = stream.CloseSend()
		return ctx.Err()
	case err := <-errc:
		if err != nil {
			// error received from frontier while waiting for harvest spec
			err = fmt.Errorf("failed to get page harvest spec: %w", err)
			span.SetTag("error", true).LogFields(tracelog.Event("error"), tracelog.Error(err))
			return err
		} else {
			// stream closed by frontier while waiting for harvest spec
			return nil
		}
	case <-waitn:
		// harvest spec received
	}

	span.SetTag("harvest.spec.uri", harvestSpec.QueuedUri.Uri).
		SetTag("harvest.spec.eid", harvestSpec.QueuedUri.ExecutionId).
		SetTag("harvest.spec.seed_uri", harvestSpec.QueuedUri.SeedUri).
		SetTag("harvest.spec.discovery_path", harvestSpec.QueuedUri.DiscoveryPath)

	log.Trace().Str("uri", harvestSpec.QueuedUri.Uri).Msg("Starting fetch")
	metrics.ActiveBrowserSessions.Inc()
	defer metrics.ActiveBrowserSessions.Dec()
	metrics.PagesTotal.Inc()
	renderResult, err := fetch(spanCtx, harvestSpec.QueuedUri, harvestSpec.CrawlConfig)
	if err != nil {
		span.SetTag("error", true).LogFields(tracelog.Event("error"), tracelog.Error(err))
		log.Error().Err(err).Msg("Fetch error")
		renderResult = &RenderResult{
			Error: errors.CommonsError(err),
		}
		metrics.PagesFailedTotal.WithLabelValues(strconv.Itoa(int(renderResult.Error.Code))).Inc()
		err := stream.Send(&frontierV1.PageHarvest{Msg: &frontierV1.PageHarvest_Error{Error: renderResult.Error}})
		if err != nil {
			if err == io.EOF {
				err = <-errc
			}
			return fmt.Errorf("failed to send error response to Frontier: %w", err)
		}
	} else {
		if renderResult.PageFetchTimeMs > 0 {
			metrics.PageFetchSeconds.Observe(float64(renderResult.PageFetchTimeMs / 1000))
		}
		err := stream.Send(&frontierV1.PageHarvest{
			Msg: &frontierV1.PageHarvest_Metrics_{
				Metrics: &frontierV1.PageHarvest_Metrics{
					UriCount:        renderResult.UriCount,
					BytesDownloaded: renderResult.BytesDownloaded,
				},
			},
		})
		if err != nil {
			if err == io.EOF {
				err = <-errc
			}
			return fmt.Errorf("failed to send metrics to Frontier: %w", err)
		}

		for _, outlink := range renderResult.Outlinks {
			err := stream.Send(&frontierV1.PageHarvest{Msg: &frontierV1.PageHarvest_Outlink{Outlink: outlink}})
			if err != nil {
				if err == io.EOF {
					err = <-errc
				}
				return fmt.Errorf("failed to send outlink to Frontier: %w", err)
			}
		}
	}

	span.SetTag("harvest.page_fetch_time_ms", renderResult.PageFetchTimeMs)
	span.SetTag("harvest.uri_count", renderResult.UriCount)
	span.SetTag("harvest.bytes_downloaded", renderResult.BytesDownloaded)
	span.SetTag("harvest.outlinks", len(renderResult.Outlinks))

	_ = stream.CloseSend()
	return <-errc
}
