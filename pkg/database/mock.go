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

package database

import (
	configV1 "github.com/nlnwa/veidemann-api/go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api/go/frontier/v1"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
	"os"
	"time"
)

type MockConnection struct {
	*connection
}

// NewMockConnection creates a new mocked connection object
func NewMockConnection() DbConnection {
	return &MockConnection{
		connection: &connection{
			dbConnectOpts: r.ConnectOpts{
				NumRetries: 10,
			},
			dbSession:    r.NewMock(),
			queryTimeout: 5 * time.Second,
			logger:       log.WithField("component", "mock connection"),
		},
	}
}

func (c *MockConnection) Close() error {
	_ = os.Remove("crawl.log")
	_ = os.Remove("page.log")
	return nil
}

func (c *MockConnection) GetMock() *r.Mock {
	return c.dbSession.(*r.Mock)
}

func (c *MockConnection) WriteCrawlLogs(crawlLogs []*frontierV1.CrawlLog) error {
	f, err := os.OpenFile("crawl.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	defer func() {
		_ = f.Close()
	}()

	for _, crawlLog := range crawlLogs {
		if _, err := f.WriteString(protojson.Format(crawlLog) + "\n"); err != nil {
			log.Println(err)
		}
	}

	return c.connection.WriteCrawlLogs(crawlLogs)
}

func (c *MockConnection) WriteCrawlLog(crawlLog *frontierV1.CrawlLog) error {
	return c.WriteCrawlLogs([]*frontierV1.CrawlLog{crawlLog})
}

func (c *MockConnection) WritePageLog(pageLog *frontierV1.PageLog) error {
	f, err := os.OpenFile("page.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := f.WriteString(protojson.Format(pageLog) + "\n"); err != nil {
		log.Println(err)
	}
	return c.connection.WritePageLog(pageLog)
}

func (c *MockConnection) GetConfig(ref *configV1.ConfigRef) (*configV1.ConfigObject, error) {
	return c.connection.GetConfig(ref)
}

func (c *MockConnection) GetConfigsForSelector(kind configV1.Kind, label *configV1.Label) ([]*configV1.ConfigObject, error) {
	return c.connection.GetConfigsForSelector(kind, label)
}
