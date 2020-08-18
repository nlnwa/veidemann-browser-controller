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

package server

import (
	"context"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/stats"
	"sync/atomic"
)

type myStatsHandler struct {
	openRPCs int32
}

func (m *myStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	return ctx
}

func (m *myStatsHandler) HandleRPC(ctx context.Context, stat stats.RPCStats) {
	switch v := stat.(type) {
	case *stats.Begin:
		atomic.AddInt32(&m.openRPCs, 1)
	case *stats.End:
		atomic.AddInt32(&m.openRPCs, -1)
		if v.Error != nil {
			log.Tracef("RPC END With error: %v", v.Error)
		}
	}
}

func (m *myStatsHandler) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	return ctx
}

func (m *myStatsHandler) HandleConn(ctx context.Context, stat stats.ConnStats) {
	switch stat.(type) {
	case *stats.ConnBegin:
	case *stats.ConnEnd:
		log.Infof("gRPC connection ended. Remaining open RPCs: %v", atomic.LoadInt32(&m.openRPCs))
	default:
	}
}
