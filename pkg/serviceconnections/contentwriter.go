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

package serviceconnections

import (
	contentwriterV1 "github.com/nlnwa/veidemann-api-go/contentwriter/v1"
	"google.golang.org/grpc"
)

// ContentWriterConn holds the client for the Content Writer service
type ContentWriterConn struct {
	opts       connectionOptions
	clientConn *grpc.ClientConn
	client     contentwriterV1.ContentWriterClient
}

func NewContentWriterConn(opts ...ConnectionOption) *ContentWriterConn {
	c := &ContentWriterConn{
		opts: defaultConnectionOptions("ContentWriter"),
	}
	for _, opt := range opts {
		opt.apply(&c.opts)
	}
	return c
}

func (c *ContentWriterConn) Connect() error {
	var err error

	// Set up ContentWriterClient
	c.clientConn, err = c.opts.connectService()
	if err != nil {
		return err
	}
	c.client = contentwriterV1.NewContentWriterClient(c.clientConn)

	return nil
}

func (c *ContentWriterConn) Close() {
	if c.clientConn != nil {
		_ = c.clientConn.Close()
	}
}

func (c *ContentWriterConn) Client() contentwriterV1.ContentWriterClient {
	return c.client
}
