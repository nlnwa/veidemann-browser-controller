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
