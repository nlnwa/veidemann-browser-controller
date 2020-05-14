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

package session

import (
	"context"
	configV1 "github.com/nlnwa/veidemann-api-go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api-go/frontier/v1"
	robotsevaluatorV1 "github.com/nlnwa/veidemann-api-go/robotsevaluator/v1"
	"github.com/nlnwa/veidemann-browser-controller/pkg/database"
)

// ParserOption configures how we parse a URL.
type SessionOption interface {
	apply(*Session)
}

// funcParserOption wraps a function that modifies parserOptions into an
// implementation of the ParserOption interface.
type funcSessionOption struct {
	f func(*Session)
}

func (fpo *funcSessionOption) apply(po *Session) {
	fpo.f(po)
}

func newFuncSessionOption(f func(*Session)) *funcSessionOption {
	return &funcSessionOption{
		f: f,
	}
}

func defaultSessionOptions() *Session {
	sess := &Session{
		browserHost:    "localhost",
		browserPort:    3000,
		browserTimeout: 500 * 1000,
		proxyPort:      3000,
		scrollPages:    20,
	}
	sess.Fetch = sess.fetch
	return sess
}

func WithBrowserHost(host string) SessionOption {
	return newFuncSessionOption(func(s *Session) {
		s.browserHost = host
	})
}

func WithBrowserPort(port int) SessionOption {
	return newFuncSessionOption(func(s *Session) {
		s.browserPort = port
	})
}

func WithProxyHost(host string) SessionOption {
	return newFuncSessionOption(func(s *Session) {
		s.proxyHost = host
	})
}

func WithProxyPort(port int) SessionOption {
	return newFuncSessionOption(func(s *Session) {
		s.proxyPort = port
	})
}

func WithDbAdapter(dbAdapter *database.DbAdapter) SessionOption {
	return newFuncSessionOption(func(s *Session) {
		s.DbAdapter = dbAdapter
	})
}

func WithFetcherFunc(f func(QUri *frontierV1.QueuedUri, crawlConfig *configV1.ConfigObject) (*RenderResult, error)) SessionOption {
	return newFuncSessionOption(func(s *Session) {
		s.Fetch = f
	})
}

func WithIsAllowedByRobotsTxtFunc(f func(ctx context.Context, request *robotsevaluatorV1.IsAllowedRequest) bool) SessionOption {
	return newFuncSessionOption(func(s *Session) {
		s.RobotsIsAllowed = f
	})
}

func WithWriteScreenshotFunc(f func(ctx context.Context, sess *Session, data []byte) error) SessionOption {
	return newFuncSessionOption(func(s *Session) {
		s.WriteScreenshot = f
	})
}

func WithScrollPages(scrollPages int) SessionOption {
	return newFuncSessionOption(func(s *Session) {
		s.scrollPages = scrollPages
	})
}
