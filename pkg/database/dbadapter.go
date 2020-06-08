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
	configV1 "github.com/nlnwa/veidemann-api-go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api-go/frontier/v1"
	log "github.com/sirupsen/logrus"
	"regexp"
	"strings"
	"sync"
	"time"
)

type entry struct {
	expires time.Time
	conf    *configV1.ConfigObject
}

type DbAdapter struct {
	db    DbConnection
	cache map[string]*entry
	ttl   time.Duration
	mu    sync.Mutex
}

func NewDbAdapter(db DbConnection, ttl time.Duration) *DbAdapter {
	return &DbAdapter{
		db:    db,
		cache: make(map[string]*entry),
		ttl:   ttl,
	}
}

func (cc *DbAdapter) GetConfigObject(ref *configV1.ConfigRef) (*configV1.ConfigObject, error) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if result, ok := cc.cache[ref.Id]; ok {
		if result.expires.After(time.Now()) {
			return result.conf, nil
		}
	}

	result, err := cc.db.GetConfig(ref)
	if err != nil {
		return nil, err
	}

	cc.cache[result.Id] = &entry{
		expires: time.Now().Add(cc.ttl),
		conf:    result,
	}

	return result, nil
}

func (cc *DbAdapter) GetScripts(browserConfig *configV1.BrowserConfig, scriptType configV1.BrowserScript_BrowserScriptType) []*configV1.BrowserScript {
	var scripts []*configV1.BrowserScript
	for _, scriptRef := range browserConfig.ScriptRef {
		if script, err := cc.GetConfigObject(scriptRef); err == nil {
			if isType(script, scriptType) {
				scripts = append(scripts, script.GetBrowserScript())
			}
		}
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	for _, selector := range browserConfig.ScriptSelector {
		t := strings.Split(selector, ":")
		label := &configV1.Label{
			Key:   t[0],
			Value: t[1],
		}
		if objs, err := cc.db.GetConfigsForSelector(configV1.Kind_browserScript, label); err == nil {
			for _, script := range objs {
				if isType(script, scriptType) {
					scripts = append(scripts, script.GetBrowserScript())
				}
				cc.cache[script.Id] = &entry{
					expires: time.Now().Add(cc.ttl),
					conf:    script,
				}
			}
		}
	}

	return scripts
}

func isType(object *configV1.ConfigObject, browserScriptType configV1.BrowserScript_BrowserScriptType) bool {
	log.Warn("got scripttype in configObj: ", object.GetBrowserScript().BrowserScriptType);
	return object.GetBrowserScript().BrowserScriptType == browserScriptType
}

func (cc *DbAdapter) GetReplacementScript(browserConfig *configV1.BrowserConfig, uri string) *configV1.BrowserScript {
	normalizedUri := NormalizeUrl(uri)
	longestMatch := 0
	var currentBestMatch *configV1.BrowserScript
	for _, bc := range cc.GetScripts(browserConfig, configV1.BrowserScript_REPLACEMENT) {
		for _, urlRegexp := range bc.UrlRegexp {
			if re, err := regexp.Compile(urlRegexp); err == nil {
				re.Longest()
				l := len(re.FindString(normalizedUri))
				if l > 0 && l > longestMatch {
					longestMatch = l
					currentBestMatch = bc
				}
			} else {
				log.Warnf("Could not match url for replacement script %v", err)
			}
		}
	}
	return currentBestMatch
}

func (cc *DbAdapter) WriteCrawlLog(crawlLog *frontierV1.CrawlLog) error {
	return cc.db.WriteCrawlLog(crawlLog)
}

func (cc *DbAdapter) WritePageLog(pageLog *frontierV1.PageLog) error {
	return cc.db.WritePageLog(pageLog)
}
