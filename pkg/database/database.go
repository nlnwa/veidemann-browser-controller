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
	"fmt"
	configV1 "github.com/nlnwa/veidemann-api-go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api-go/frontier/v1"
	log "github.com/sirupsen/logrus"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

type DbConnection interface {
	Connect() error
	GetConfig(ref *configV1.ConfigRef) (*configV1.ConfigObject, error)
	GetConfigsForSelector(kind configV1.Kind, label *configV1.Label) ([]*configV1.ConfigObject, error)
	WriteCrawlLog(crawlLog *frontierV1.CrawlLog) error
	WritePageLog(pageLog *frontierV1.PageLog) error
}

// connection holds the connections for ContentWriter and Veidemann database
type connection struct {
	dbConnectOpts r.ConnectOpts
	dbSession     r.QueryExecutor
}

// NewConnection creates a new connection object
func NewConnection(dbHost string, dbPort int, dbUser string, dbPassword string, dbName string) DbConnection {
	c := &connection{
		dbConnectOpts: r.ConnectOpts{
			Address:    fmt.Sprintf("%s:%d", dbHost, dbPort),
			Username:   dbUser,
			Password:   dbPassword,
			Database:   dbName,
			NumRetries: 10,
		},
	}
	return c
}

// connect establishes connections
func (c *connection) Connect() error {
	// Set up database connection
	dbSession, err := r.Connect(c.dbConnectOpts)
	if err != nil {
		log.Errorf("fail to connect to database: %v", err)
		return err
	}
	c.dbSession = dbSession

	log.Infof("Recorder Proxy is using DB at: %s", c.dbConnectOpts.Address)
	return nil
}

func (c *connection) GetConfig(ref *configV1.ConfigRef) (*configV1.ConfigObject, error) {
	res, err := r.Table("config").Get(ref.Id).Run(c.dbSession)
	if err != nil {
		return nil, err
	}
	defer res.Close()
	var result configV1.ConfigObject
	err = res.One(&result)

	if err != nil {
		return nil, fmt.Errorf("DB error: %w", err)
	}
	return &result, nil
}

func (c *connection) GetConfigsForSelector(kind configV1.Kind, label *configV1.Label) ([]*configV1.ConfigObject, error) {
	res, err := r.Table("config").GetAllByIndex("label", r.Expr([]string{label.Key, label.Value})).
		Filter(func(row r.Term) r.Term {
			return row.Field("kind").Eq(kind.String())
		}).
		Run(c.dbSession)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = res.Close()
	}()

	var configObject configV1.ConfigObject
	var configObjects []*configV1.ConfigObject
	for res.Next(&configObject) {
		//noinspection GoVetCopyLock
		aCopy := configObject
		configObjects = append(configObjects, &aCopy)
	}
	if err := res.Err(); err != nil {
		return nil, fmt.Errorf("failed to fetch config from cursor: %v", err)
	}

	return configObjects, nil
}

func (c *connection) WriteCrawlLog(crawlLog *frontierV1.CrawlLog) error {
	_, err := r.Table("crawl_log").Insert(crawlLog).RunWrite(c.dbSession)
	return err
}

func (c *connection) WritePageLog(pageLog *frontierV1.PageLog) error {
	_, err := r.Table("page_log").Insert(pageLog).RunWrite(c.dbSession)
	return err
}
