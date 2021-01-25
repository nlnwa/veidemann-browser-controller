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
	"fmt"
	configV1 "github.com/nlnwa/veidemann-api/go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api/go/frontier/v1"
	"github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
	"time"
)

// connection holds the connections for ContentWriter and Veidemann database
type connection struct {
	dbConnectOpts      r.ConnectOpts
	dbSession          r.QueryExecutor
	maxRetries         int
	waitTimeout        time.Duration
	queryTimeout       time.Duration
	maxOpenConnections int
	logger             *log.Entry
}

type Options struct {
	Username           string
	Password           string
	Database           string
	UseOpenTracing     bool
	Host               string
	Port               int
	QueryTimeout       time.Duration
	MaxOpenConnections int
}

// NewConnection creates a new connection object
func NewConnection(opts Options) DbConnection {
	return &connection{
		dbConnectOpts: r.ConnectOpts{
			Address:        fmt.Sprintf("%s:%d", opts.Host, opts.Port),
			Username:       opts.Username,
			Password:       opts.Password,
			Database:       opts.Database,
			InitialCap:     2,
			MaxOpen:        opts.MaxOpenConnections,
			UseOpentracing: opts.UseOpenTracing,
			NumRetries:     10,
			Timeout:        10 * time.Second,
		},
		maxRetries:   3,
		waitTimeout:  60 * time.Second,
		queryTimeout: time.Duration(opts.QueryTimeout) * time.Second,
		logger:       log.WithField("component", "database"),
	}
}

// Connect establishes connections
func (c *connection) Connect() error {
	var err error
	// Set up database connection
	c.dbSession, err = r.Connect(c.dbConnectOpts)
	if err != nil {
		return fmt.Errorf("failed to connect to RethinkDB at %s: %w", c.dbConnectOpts.Address, err)
	}

	c.logger.Infof("Connected to RethinkDB at %s", c.dbConnectOpts.Address)
	return nil
}

// Close closes the connection
func (c *connection) Close() error {
	c.logger.Infof("Closing database connection")
	return c.dbSession.(*r.Session).Close()
}

func (c *connection) GetConfig(ref *configV1.ConfigRef) (*configV1.ConfigObject, error) {
	term := r.Table("config").Get(ref.Id)
	res, err := c.execRead("get-config-object", &term)
	if err != nil {
		return nil, err
	}
	var result configV1.ConfigObject
	err = res.One(&result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *connection) GetConfigsForSelector(kind configV1.Kind, label *configV1.Label) ([]*configV1.ConfigObject, error) {
	term := r.Table("config").GetAllByIndex("label", r.Expr([]string{label.Key, label.Value})).
		Filter(func(row r.Term) r.Term {
			return row.Field("kind").Eq(kind.String())
		})
	res, err := c.execRead("get-configs-by-label", &term)
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
		return nil, err
	}

	return configObjects, nil
}

func (c *connection) WriteCrawlLog(crawlLog *frontierV1.CrawlLog) error {
	return c.WriteCrawlLogs([]*frontierV1.CrawlLog{crawlLog})
}

func (c *connection) WriteCrawlLogs(crawlLogs []*frontierV1.CrawlLog) error {
	term := r.Table("crawl_log").Insert(crawlLogs)
	return c.execWrite("write-crawl-log(s)", &term)
}

func (c *connection) WritePageLog(pageLog *frontierV1.PageLog) error {
	term := r.Table("page_log").Insert(pageLog)
	return c.execWrite("write-page-log", &term)
}

// execRead executes the given read term with a timeout
func (c *connection) execRead(name string, term *r.Term) (*r.Cursor, error) {
	q := func(ctx context.Context) (*r.Cursor, error) {
		runOpts := r.RunOpts{
			Context: ctx,
		}
		return term.Run(c.dbSession, runOpts)
	}
	return c.execWithRetry(name, q)
}

// execWrite executes the given write term with a timeout
func (c *connection) execWrite(name string, term *r.Term) error {
	q := func(ctx context.Context) (*r.Cursor, error) {
		runOpts := r.RunOpts{
			Context:    ctx,
			Durability: "soft",
		}
		_, err := (*term).RunWrite(c.dbSession, runOpts)
		return nil, err
	}
	_, err := c.execWithRetry(name, q)
	return err
}

// execWithRetry executes given query function repeatedly until successful or max retry limit is reached
func (c *connection) execWithRetry(name string, q func(ctx context.Context) (*r.Cursor, error)) (cursor *r.Cursor, err error) {
	attempts := 0
out:
	for {
		attempts++
		cursor, err = c.exec(name, q)
		if err == nil {
			return
		}
		c.logger.WithError(err).
			WithField("operation", name).
			WithField("retries", attempts-1).
			Warn()
		switch err {
		case r.ErrQueryTimeout:
			err := c.wait()
			if err != nil {
				c.logger.WithError(err).Warn()
			}
		case r.ErrConnectionClosed:
			err := c.Connect()
			if err != nil {
				c.logger.WithError(err).Warn()
			}
		default:
			break out
		}
		if attempts > c.maxRetries {
			break
		}
	}
	return nil, fmt.Errorf("failed to %s after %d of %d attempts: %w", name, attempts, c.maxRetries+1, err)
}

// exec executes the given query using a timeout
//
// A tracing span is created if configured
func (c *connection) exec(name string, q func(ctx context.Context) (*r.Cursor, error)) (*r.Cursor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	if c.dbConnectOpts.UseOpentracing {
		span := opentracing.GlobalTracer().StartSpan(name)
		ctx = opentracing.ContextWithSpan(ctx, span)
		defer span.Finish()
	}
	return q(ctx)
}

// wait waits for database to be fully up date and ready for read/write
func (c *connection) wait() error {
	ctx, cancel := context.WithTimeout(context.Background(), (1*time.Second)+c.waitTimeout)
	defer cancel()
	runOpts := r.RunOpts{
		Context: ctx,
	}
	waitOpts := r.WaitOpts{
		Timeout: c.waitTimeout,
	}
	_, err := r.DB(c.dbConnectOpts.Database).Wait(waitOpts).Run(c.dbSession, runOpts)
	return err
}
