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
	"github.com/nlnwa/veidemann-api-go/frontier/v1"
)

type Request struct {
	Method         string
	Url            string
	RequestId      string
	NetworkId      string
	CrawlLog       *frontier.CrawlLog
	GotNew         bool
	GotComplete    bool
	Initiator      string
	ResourceType   string
	Referrer       string
	RedirectParent *Request
	FromCache      bool
	////final BaseSpan parentSpan;
	////Span span;

	// True if this request is for the top level request.
	// It is also true if the request is a redirect from the top level request.
	rootResource bool
}
