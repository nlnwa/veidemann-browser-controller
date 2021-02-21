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
	"context"
	configV1 "github.com/nlnwa/veidemann-api/go/config/v1"
	log "github.com/sirupsen/logrus"
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
			batchSize: 200,
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

func (c *MockConnection) GetConfig(ctx context.Context, ref *configV1.ConfigRef) (*configV1.ConfigObject, error) {
	return c.connection.GetConfig(ctx, ref)
}

func (c *MockConnection) GetConfigsForSelector(ctx context.Context, kind configV1.Kind, label *configV1.Label) ([]*configV1.ConfigObject, error) {
	return c.connection.GetConfigsForSelector(ctx, kind, label)
}
