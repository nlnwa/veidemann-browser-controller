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
	"github.com/golang/protobuf/ptypes"
	configV1 "github.com/nlnwa/veidemann-api/go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api/go/frontier/v1"
	"strings"
	"sync"
	"time"
)

type entry struct {
	expires time.Time
	configs []*configV1.ConfigObject
}

type cache struct {
	entries map[string]*entry
	ttl     time.Duration
	mu      sync.RWMutex
}

func newCache(ttl time.Duration) *cache {
	c := &cache{
		entries: make(map[string]*entry),
		ttl:     ttl,
	}
	go func() {
		for {
			c.purge()
			time.Sleep(ttl + 1*time.Minute)
		}
	}()
	return c
}

func (c *cache) purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range c.entries {
		if entry != nil && entry.expires.Before(time.Now()) {
			delete(c.entries, key)
		}
	}
}

func (c *cache) Set(key string, value *configV1.ConfigObject) {
	c.SetMany(key, []*configV1.ConfigObject{value})
}

func (c *cache) SetMany(key string, values []*configV1.ConfigObject) {
	if c.ttl == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &entry{
		expires: time.Now().Add(c.ttl),
		configs: values,
	}
}

func (c *cache) Get(key string) *configV1.ConfigObject {
	configs := c.GetMany(key)
	if len(configs) > 0 {
		return configs[0]
	}
	return nil
}

func (c *cache) GetMany(key string) []*configV1.ConfigObject {
	if c.ttl == 0 {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if result, ok := c.entries[key]; ok {
		if result.expires.After(time.Now()) {
			return result.configs
		}
	}
	return nil
}

type DbConnection interface {
	Connect() error
	Close() error
	GetConfig(ctx context.Context, ref *configV1.ConfigRef) (*configV1.ConfigObject, error)
	GetConfigsForSelector(ctx context.Context, kind configV1.Kind, label *configV1.Label) ([]*configV1.ConfigObject, error)
	WriteCrawlLog(ctx context.Context, crawlLog *frontierV1.CrawlLog) error
	WriteCrawlLogs(ctx context.Context, crawlLogs []*frontierV1.CrawlLog) error
	WritePageLog(ctx context.Context, pageLog *frontierV1.PageLog) error
}

type DbAdapter struct {
	db    DbConnection
	cache *cache
	ttl   time.Duration
}

func NewDbAdapter(db DbConnection, ttl time.Duration) *DbAdapter {
	return &DbAdapter{
		db:    db,
		cache: newCache(ttl),
	}
}

func (cc *DbAdapter) GetConfigObject(ctx context.Context, ref *configV1.ConfigRef) (*configV1.ConfigObject, error) {
	cached := cc.cache.Get(ref.Id)
	if cached != nil {
		return cached, nil
	}

	result, err := cc.db.GetConfig(ctx, ref)
	if err != nil {
		return nil, err
	}

	cc.cache.Set(result.Id, result)

	return result, nil
}

// fetch configObjects by selector string (key:value)
func (cc *DbAdapter) getConfigsForSelector(ctx context.Context, selector string) ([]*configV1.ConfigObject, error) {
	cached := cc.cache.GetMany(selector)
	if cached != nil {
		return cached, nil
	}

	t := strings.Split(selector, ":")
	label := &configV1.Label{
		Key:   t[0],
		Value: t[1],
	}

	res, err := cc.db.GetConfigsForSelector(ctx, configV1.Kind_browserScript, label)
	if err != nil {
		return nil, err
	}
	cc.cache.SetMany(selector, res)

	return res, nil
}

func (cc *DbAdapter) GetScripts(ctx context.Context, browserConfig *configV1.BrowserConfig) ([]*configV1.ConfigObject, error) {
	var scripts []*configV1.ConfigObject
	for _, scriptRef := range browserConfig.ScriptRef {
		script, err := cc.GetConfigObject(ctx, scriptRef)
		if err != nil {
			return nil, fmt.Errorf("failed to get script by reference %v: %w", scriptRef, err)
		}
		scripts = append(scripts, script)
	}
	for _, selector := range browserConfig.ScriptSelector {
		configs, err := cc.getConfigsForSelector(ctx, selector)
		if err != nil {
			return nil, fmt.Errorf("failed to get scripts by selector %s: %w", selector, err)
		}
		for _, config := range configs {
			scripts = append(scripts, config)
		}
	}
	return scripts, nil
}

func (cc *DbAdapter) WriteCrawlLog(ctx context.Context, crawlLog *frontierV1.CrawlLog) error {
	return cc.WriteCrawlLogs(ctx, []*frontierV1.CrawlLog{crawlLog})
}

func (cc *DbAdapter) WriteCrawlLogs(ctx context.Context, crawlLogs []*frontierV1.CrawlLog) error {
	for _, crawlLog := range crawlLogs {
		crawlLog.TimeStamp = ptypes.TimestampNow()
	}
	return cc.db.WriteCrawlLogs(ctx, crawlLogs)
}

func (cc *DbAdapter) WritePageLog(ctx context.Context, pageLog *frontierV1.PageLog) error {
	return cc.db.WritePageLog(ctx, pageLog)
}
