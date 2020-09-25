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
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type ClientConn struct {
	conn *grpc.ClientConn
	opts *connectionOptions
}

func (c *ClientConn) Connect() error {
	var err error
	c.conn, err = c.opts.connectService()
	return err
}

func (c *ClientConn) Close() {
	if c.conn != nil {
		err := c.conn.Close()
		if err == nil {
			log.Infof("Disconnected from %v", c.opts.serviceName)
		} else {
			log.WithError(err).Warnf("Error while disconnecting from %v", c.opts.serviceName)
		}
	}
}

func (c *ClientConn) Connection() *grpc.ClientConn {
	return c.conn
}

func NewClientConn(serviceName string, opts ...ConnectionOption) *ClientConn {
	o := defaultConnectionOptions(serviceName)
	for _, opt := range opts {
		opt.apply(&o)
	}

	return &ClientConn{
		opts: &o,
	}
}
