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
	"github.com/nlnwa/veidemann-browser-controller/pkg/errors"
	"github.com/nlnwa/veidemann-browser-controller/pkg/metrics"
	"github.com/nlnwa/veidemann-browser-controller/pkg/serviceconnections"
	log "github.com/sirupsen/logrus"
	"io"
	"strconv"
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
	Connect() error
	Close()
	Harvest(context.Context, FetchFunc) error
}

type harvester struct {
	clientConn *serviceconnections.ClientConn
	client     frontierV1.FrontierClient
}

func New(opts ...serviceconnections.ConnectionOption) Harvester {
	return &harvester{clientConn: serviceconnections.NewClientConn("Frontier", opts...)}
}

func (h *harvester) Connect() error {
	if err := h.clientConn.Connect(); err != nil {
		return err
	} else {
		h.client = frontierV1.NewFrontierClient(h.clientConn.Connection())
		return nil
	}
}

func (h *harvester) Close() {
	h.clientConn.Close()
}

func (h *harvester) Harvest(ctx context.Context, fetch FetchFunc) error {
	harvestCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := h.client.GetNextPage(harvestCtx)
	if err != nil {
		return fmt.Errorf("failed to get the frontier GetNextPage client: %w", err)
	}

	err = stream.Send(&frontierV1.PageHarvest{Msg: &frontierV1.PageHarvest_RequestNextPage{RequestNextPage: true}})
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to send request for new page to frontier: %w", err)
	}
	var harvestSpec *frontierV1.PageHarvestSpec
	harvestSpec, err = stream.Recv()
	if err != nil {
		return fmt.Errorf("failed to get page harvest spec: %w", err)
	}

	errc := make(chan error)
	go func() {
		for {
			_, err := stream.Recv()
			if err == io.EOF {
				// stream completed
				close(errc)
				return
			}
			if err != nil {
				errc <- err
				return
			}
		}
	}()

	log.WithField("uri", harvestSpec.QueuedUri.Uri).Tracef("Starting fetch")
	metrics.ActiveBrowserSessions.Inc()
	defer metrics.ActiveBrowserSessions.Dec()
	metrics.PagesTotal.Inc()
	renderResult, err := fetch(ctx, harvestSpec.QueuedUri, harvestSpec.CrawlConfig)
	if err != nil {
		log.Errorf("Failed to fetch: %v", err)
		renderResult = &RenderResult{
			Error: errors.CommonsError(err),
		}
		metrics.PagesFailedTotal.WithLabelValues(strconv.Itoa(int(renderResult.Error.Code))).Inc()
		err := stream.Send(&frontierV1.PageHarvest{Msg: &frontierV1.PageHarvest_Error{Error: renderResult.Error}})
		if err != nil {
			if err != io.EOF {
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

	if err := stream.CloseSend(); err != nil {
		return fmt.Errorf("failed to close send: %w", err)
	}
	return <-errc
}
