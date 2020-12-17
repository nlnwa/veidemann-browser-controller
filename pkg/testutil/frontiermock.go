/*
 * Copyright 2019 National Library of Norway.
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

package testutil

import (
	"context"
	"fmt"
	commonsV1 "github.com/nlnwa/veidemann-api/go/commons/v1"
	configV1 "github.com/nlnwa/veidemann-api/go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api/go/frontier/v1"
	"github.com/nlnwa/veidemann-recorderproxy/serviceconnections"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"io"
	"net"
	"sync"
)

const bufSize = 1024 * 1024

type RequestPageFunc func() (qUri *frontierV1.QueuedUri, crawlConfig *configV1.ConfigObject)
type ErrorFunc func(*commonsV1.Error)
type MetricsFunc func(*frontierV1.PageHarvest_Metrics)
type OutlinkFunc func(*frontierV1.QueuedUri)

/**
 * Server mocks
 */
type FrontierMock struct {
	frontierV1.UnimplementedFrontierServer
	lis             *bufconn.Listener
	l               *sync.Mutex
	contextDialer   grpc.DialOption
	Server          *grpc.Server
	ClientConn      *serviceconnections.Connections
	requestPageFunc RequestPageFunc
	errorFunc       ErrorFunc
	metricsFunc     MetricsFunc
	outlinkFunc     OutlinkFunc
}

func NewFrontierMock(requestPageFunc RequestPageFunc, errorFunc ErrorFunc, metricsFunc MetricsFunc, outlinkFunc OutlinkFunc) *FrontierMock {
	m := &FrontierMock{
		lis:             bufconn.Listen(bufSize),
		l:               &sync.Mutex{},
		requestPageFunc: requestPageFunc,
		errorFunc:       errorFunc,
		metricsFunc:     metricsFunc,
		outlinkFunc:     outlinkFunc,
	}

	m.contextDialer = grpc.WithContextDialer(m.bufDialer)

	m.Server = grpc.NewServer()

	frontierV1.RegisterFrontierServer(m.Server, m)

	go func() {
		if err := m.Server.Serve(m.lis); err != nil {
			log.Fatalf("Server exited with error: %v", err)
		}
	}()

	return m
}

func (m *FrontierMock) Close() {
	m.ClientConn.Close()
	m.Server.GracefulStop()
	_ = m.lis.Close()
}

func (m *FrontierMock) BufDialerOption() grpc.DialOption {
	return grpc.WithContextDialer(m.bufDialer)
}

func (m *FrontierMock) bufDialer(context.Context, string) (net.Conn, error) {
	return m.lis.Dial()
}

// Implements Frontier
func (m *FrontierMock) CrawlSeed(context.Context, *frontierV1.CrawlSeedRequest) (*frontierV1.CrawlExecutionId, error) {
	panic("implement me")
}

// Implements Frontier
func (m *FrontierMock) GetNextPage(server frontierV1.Frontier_GetNextPageServer) error {
	for {
		in, err := server.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch v := in.Msg.(type) {
		case *frontierV1.PageHarvest_RequestNextPage:
			qUri, crawlConfig := m.requestPageFunc()
			if err := server.Send(&frontierV1.PageHarvestSpec{QueuedUri: qUri, CrawlConfig: crawlConfig}); err != nil {
				fmt.Printf("xerr: %v\n", err)
			}
		case *frontierV1.PageHarvest_Error:
			m.errorFunc(v.Error)
		case *frontierV1.PageHarvest_Metrics_:
			m.metricsFunc(v.Metrics)
		case *frontierV1.PageHarvest_Outlink:
			m.outlinkFunc(v.Outlink)
		default:
			fmt.Printf("MSG: %T %v\n", v, v)
		}
	}
}
