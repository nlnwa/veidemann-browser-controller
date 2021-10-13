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

package frontier

import (
	"context"
	"fmt"
	commonsV1 "github.com/nlnwa/veidemann-api/go/commons/v1"
	frontierV1 "github.com/nlnwa/veidemann-api/go/frontier/v1"
	"github.com/nlnwa/veidemann-browser-controller/errors"
	"github.com/nlnwa/veidemann-browser-controller/metrics"
	"github.com/nlnwa/veidemann-browser-controller/serviceconnections"
	"github.com/opentracing/opentracing-go"
	tracelog "github.com/opentracing/opentracing-go/log"
	"google.golang.org/protobuf/types/known/emptypb"
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

// type FetchFunc func(context.Context, *frontierV1.QueuedUri, *configV1.ConfigObject) (*RenderResult, error)

type Frontier interface {
	serviceconnections.Connection
	GetNextPage(context.Context) (*frontierV1.PageHarvestSpec, error)
	PageCompleted(*frontierV1.PageHarvestSpec, *RenderResult, error) error
}

type frontier struct {
	*serviceconnections.ClientConn
	frontierV1.FrontierClient
}

func New(opts ...serviceconnections.ConnectionOption) Frontier {
	return &frontier{
		ClientConn: serviceconnections.NewClientConn("Frontier", opts...),
	}
}

func (f *frontier) Connect() error {
	if err := f.ClientConn.Connect(); err != nil {
		return err
	} else {
		f.FrontierClient = frontierV1.NewFrontierClient(f.ClientConn.Connection())
		return nil
	}
}

func (f *frontier) GetNextPage(ctx context.Context) (*frontierV1.PageHarvestSpec, error) {
	span, spanCtx := opentracing.StartSpanFromContext(ctx, "get-next-page")
	defer span.Finish()

	// get page to fetch
	harvestSpec, err := f.FrontierClient.GetNextPage(spanCtx, new(emptypb.Empty))
	if err != nil {
		return nil, err
	}
	return harvestSpec, nil
}

func (f *frontier) PageCompleted(phs *frontierV1.PageHarvestSpec, renderResult *RenderResult, fetchErr error) error {
	span := opentracing.StartSpan("page-completed")
	defer span.Finish()

	ctx := opentracing.ContextWithSpan(context.Background(), span)

	defer metrics.PagesTotal.Inc()

	stream, err := f.FrontierClient.PageCompleted(ctx)
	if err != nil {
		return err
	}

	if fetchErr != nil {
		renderResult = &RenderResult{
			Error: errors.CommonsError(fetchErr),
		}

		span.SetTag("error", true).LogFields(tracelog.Event("error"), tracelog.Error(fetchErr))
		metrics.PagesFailedTotal.WithLabelValues(strconv.Itoa(int(renderResult.Error.Code))).Inc()

		if err := stream.Send(&frontierV1.PageHarvest{
			Msg:          &frontierV1.PageHarvest_Error{Error: renderResult.Error},
			SessionToken: phs.SessionToken,
		}); err != nil {
			if err == io.EOF {
				_, err = stream.CloseAndRecv()
			}
			return fmt.Errorf("failed to send fetch error response to Frontier: %w", err)
		}
		_, err := stream.CloseAndRecv()
		return err
	}

	// update trace span
	span.SetTag("harvest.page_fetch_time_ms", renderResult.PageFetchTimeMs)
	span.SetTag("harvest.uri_count", renderResult.UriCount)
	span.SetTag("harvest.bytes_downloaded", renderResult.BytesDownloaded)
	span.SetTag("harvest.outlinks", len(renderResult.Outlinks))

	// update metrics
	if renderResult.PageFetchTimeMs > 0 {
		metrics.PageFetchSeconds.Observe(float64(renderResult.PageFetchTimeMs / 1000))
	}

	// send harvest metrics
	if err := stream.Send(&frontierV1.PageHarvest{
		Msg: &frontierV1.PageHarvest_Metrics_{
			Metrics: &frontierV1.PageHarvest_Metrics{
				UriCount:        renderResult.UriCount,
				BytesDownloaded: renderResult.BytesDownloaded,
			},
		},
		SessionToken: phs.SessionToken,
	}); err != nil {
		if err == io.EOF {
			_, err = stream.CloseAndRecv()
		}
		span.SetTag("error", true).LogFields(tracelog.Event("error"), tracelog.Error(err))
		return fmt.Errorf("failed to send metrics to Frontier: %w", err)
	}

	// send harvest outlinks
	for _, outlink := range renderResult.Outlinks {
		err := stream.Send(&frontierV1.PageHarvest{
			Msg:          &frontierV1.PageHarvest_Outlink{Outlink: outlink},
			SessionToken: phs.SessionToken,
		})
		if err != nil {
			if err == io.EOF {
				_, err = stream.CloseAndRecv()
			}
			span.SetTag("error", true).LogFields(tracelog.Event("error"), tracelog.Error(err))
			return fmt.Errorf("failed to send outlink to Frontier: %w", err)
		}
	}

	_, err = stream.CloseAndRecv()
	return err
}
