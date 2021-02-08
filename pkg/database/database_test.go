package database

import (
	"context"
	"fmt"
	"github.com/nlnwa/veidemann-api/go/frontier/v1"
	log "github.com/sirupsen/logrus"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
	"math/rand"
	"testing"
	"time"
)

func Test_WriteCrawlLogs(t *testing.T) {
	dbConnectionOpts := r.ConnectOpts{
		Username:       "admin",
		Password:       "rethinkdb",
		Database:       "veidemann",
		Address:        "localhost:28015",
		InitialCap:     2,
		MaxOpen:        10,
		UseOpentracing: false,
		NumRetries:     10,
		Timeout:        10 * time.Second,
	}
	for round := 0; round < 50; round++ {
		mock := r.NewMock(dbConnectionOpts)
		nrOfCrawlLogs := rand.Intn(100) + 1
		batchSize := rand.Intn(28) + 1

		conn := &connection{
			dbConnectOpts:      dbConnectionOpts,
			dbSession:          mock,
			maxRetries:         5,
			waitTimeout:        1 * time.Minute,
			queryTimeout:       1 * time.Minute,
			maxOpenConnections: 2,
			logger:             log.WithField("component", "test"),
			batchSize:          batchSize,
		}
		nrOfBatches := nrOfCrawlLogs / batchSize
		if nrOfCrawlLogs%batchSize > 0 {
			nrOfBatches += 1
		}
		batches := make([][]*frontier.CrawlLog, nrOfBatches)
		var crawlLogs []*frontier.CrawlLog
		for i := 0; i < nrOfBatches; i++ {
			for j := 0; j < batchSize; j++ {
				if i*batchSize+j >= nrOfCrawlLogs {
					break
				}
				crawlLog := &frontier.CrawlLog{WarcId: fmt.Sprintf("%d:%d", i, j)}
				batches[i] = append(batches[i], crawlLog)
				crawlLogs = append(crawlLogs, crawlLog)
			}
			resp := []interface{}{r.WriteResponse{
				Inserted: len(batches[i]),
			}}
			mock.On(r.Table("crawl_log").Insert(batches[i])).Return(resp, nil)
		}

		err := conn.WriteCrawlLogs(context.TODO(), crawlLogs)
		if err != nil {
			t.Errorf("%v", err)
		}
		mock.AssertExpectations(t)
	}
}
