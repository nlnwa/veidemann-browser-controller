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
	"context"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"time"
)

// connectionOptions configure a connection. connectionOptions are set by the ConnectionOption
// values passed to NewConnectionOptions.
type connectionOptions struct {
	serviceName    string
	host           string
	port           string
	connectTimeout time.Duration
	dialOptions    []grpc.DialOption
}

func (c *connectionOptions) Addr() string {
	return c.host + ":" + c.port
}

// ConnectionOption configures how to connect to a service.
type ConnectionOption interface {
	apply(*connectionOptions)
}

// EmptyConnectionOption does not alter the configuration. It can be embedded in
// another structure to build custom connection options.
type EmptyConnectionOption struct{}

func (EmptyConnectionOption) apply(*connectionOptions) {}

// funcConnectionOption wraps a function that modifies connectionOptions into an
// implementation of the ConnectionOption interface.
type funcConnectionOption struct {
	f func(*connectionOptions)
}

func (fco *funcConnectionOption) apply(po *connectionOptions) {
	fco.f(po)
}

func newFuncConnectionOption(f func(*connectionOptions)) *funcConnectionOption {
	return &funcConnectionOption{
		f: f,
	}
}

func defaultConnectionOptions(serviceName string) connectionOptions {
	return connectionOptions{
		serviceName:    serviceName,
		connectTimeout: 10 * time.Minute,
	}
}

func (opts *connectionOptions) connectService() (*grpc.ClientConn, error) {
	log.Infof("Connecting %s at: %s", opts.serviceName, opts.Addr())

	dialOpts := append(opts.dialOptions,
		grpc.WithInsecure(),
		grpc.WithBlock(),
	)

	dialCtx, dialCancel := context.WithTimeout(context.Background(), opts.connectTimeout)
	defer dialCancel()

	return grpc.DialContext(dialCtx, opts.Addr(), dialOpts...)
}

func WithHost(host string) ConnectionOption {
	return newFuncConnectionOption(func(c *connectionOptions) {
		c.host = host
	})
}

func WithPort(port string) ConnectionOption {
	return newFuncConnectionOption(func(c *connectionOptions) {
		c.port = port
	})
}

func WithDialOptions(dialOption ...grpc.DialOption) ConnectionOption {
	return newFuncConnectionOption(func(c *connectionOptions) {
		c.dialOptions = dialOption
	})
}

func WithConnectTimeout(connectTimeout time.Duration) ConnectionOption {
	return newFuncConnectionOption(func(c *connectionOptions) {
		c.connectTimeout = connectTimeout
	})
}
