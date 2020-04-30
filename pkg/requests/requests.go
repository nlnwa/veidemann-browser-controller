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

package requests

import (
	"context"
	"github.com/nlnwa/veidemann-api-go/frontier/v1"
	"github.com/nlnwa/veidemann-browser-controller/pkg/syncx"
	log "github.com/sirupsen/logrus"
	"sync"
)

type RequestRegistry interface {
	NotifyLoadStart()
	NotifyLoadFinished()
	AddRequest(req *Request)
	GetByNetworkId(id string) *Request
	GetByRequestId(id string) *Request
	GetByUrl(url string, onlyNew bool) *Request
	MatchCrawlLogs() bool
	Walk(w func(*Request))
	InitialRequest() *Request
	RootRequest() *Request
	FinalizeResponses(requestedUrl *frontier.QueuedUri)
}

type requestRegistry struct {
	ctx         context.Context
	done        *syncx.WaitGroup
	mu          sync.Mutex
	requests    []*Request
	rootRequest *Request
}

func (r *requestRegistry) InitialRequest() *Request {
	return r.requests[0]
}

func (r *requestRegistry) RootRequest() *Request {
	return r.rootRequest
}

func NewRegistry(ctx context.Context, done *syncx.WaitGroup) RequestRegistry {
	r := &requestRegistry{
		ctx:  ctx,
		done: done,
	}

	return r
}

func (r *requestRegistry) NotifyLoadStart() {
	r.done.Add(1)
}

func (r *requestRegistry) NotifyLoadFinished() {
	r.done.Done()
}

func (r *requestRegistry) AddRequest(req *Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
}

func (r *requestRegistry) GetByNetworkId(id string) *Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, req := range r.requests {
		if req.NetworkId == id {
			return req
		}
	}
	return nil
}

func (r *requestRegistry) GetByRequestId(id string) *Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, req := range r.requests {
		if req.RequestId == id {
			return req
		}
	}
	return nil
}

func (r *requestRegistry) GetByUrl(url string, onlyNew bool) *Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, req := range r.requests {
		if req.Url == url {
			if !onlyNew || !req.GotNew {
				return req
			}
		}
	}
	return nil
}

func (r *requestRegistry) MatchCrawlLogs() bool {
	unresolved := 0
	for _, l := range r.requests {
		if l.CrawlLog == nil {
			unresolved++
			log.Tracef("Missing crawllog for %v -- %v %v", l.RequestId, l.GotNew, l.GotComplete)
		} else {
			log.Tracef("found crawllog for %v -- %v %v", l.RequestId, l.GotNew, l.GotComplete)
		}
	}
	if unresolved > 0 {
		return false
	}
	return true
}

func (r *requestRegistry) Walk(w func(*Request)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, req := range r.requests {
		w(req)
	}
}

func (r *requestRegistry) FinalizeResponses(requestedUrl *frontier.QueuedUri) {
	urls := make(map[string]*Request)
	ids := make(map[string]*Request)
	r.rootRequest = r.requests[0]
	for idx, rr := range r.requests {
		urls[rr.Url] = rr
		if p, ok := ids[rr.NetworkId]; ok {
			rr.RedirectParent = p
			if p == r.rootRequest {
				r.rootRequest = rr
			}
		}
		ids[rr.NetworkId] = rr

		if rr.CrawlLog != nil {
			rr.CrawlLog.Referrer = rr.Referrer
			referrerRequest := urls[rr.Referrer]

			var discoveryType string
			if idx == 0 {
				discoveryType = requestedUrl.DiscoveryPath
			} else if rr.Initiator == "script" {
				// Resource is loaded by a script
				discoveryType = "X"
			} else if rr.RedirectParent != nil {
				discoveryType = "R"
			} else {
				discoveryType = "E"
			}

			if rr.RedirectParent != nil && rr.RedirectParent.CrawlLog != nil {
				rr.CrawlLog.DiscoveryPath = rr.RedirectParent.CrawlLog.DiscoveryPath + discoveryType
			} else if referrerRequest != nil && referrerRequest.CrawlLog != nil {
				rr.CrawlLog.DiscoveryPath = referrerRequest.CrawlLog.DiscoveryPath + discoveryType
			} else {
				rr.CrawlLog.DiscoveryPath = discoveryType
			}
		} else {
			log.Warnf("CrawlLog missing: %v %v %v %v %v %v\n", idx, rr.NetworkId, rr.GotNew, rr.GotComplete, rr.RequestId, rr.Url)
		}
	}
}
