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

package controller

import (
	"github.com/nlnwa/veidemann-browser-controller/pkg/serviceconnections"
	"github.com/nlnwa/veidemann-browser-controller/pkg/session"
	"time"
)

// browserControllerOptions configure the BrowserController. browserControllerOptions are set by the BrowserControllerOption
// values passed to New.
type browserControllerOptions struct {
	listenInterface     string
	listenPort          int
	frontierConn        *serviceconnections.FrontierConn
	robotsEvaluatorConn *serviceconnections.RobotsEvaluatorConn
	contentWriterConn   *serviceconnections.ContentWriterConn
	sessionOpts         []session.SessionOption
	maxSessions         int
	closeTimeout        time.Duration
}

// BrowserControllerOption configures BrowserController.
type BrowserControllerOption interface {
	apply(*browserControllerOptions)
}

// funcBrowserControllerOption wraps a function that modifies browserControllerOptions into an
// implementation of the BrowserControllerOption interface.
type funcBrowserControllerOption struct {
	f func(*browserControllerOptions)
}

func (fco *funcBrowserControllerOption) apply(po *browserControllerOptions) {
	fco.f(po)
}

func newFuncBrowserControllerOption(f func(*browserControllerOptions)) *funcBrowserControllerOption {
	return &funcBrowserControllerOption{
		f: f,
	}
}

func defaultBrowserControllerOptions() browserControllerOptions {
	return browserControllerOptions{
		closeTimeout:    5 * time.Minute,
		maxSessions:     1,
		listenInterface: "",
		listenPort:      8080,
	}
}

func WithListenInterface(listenInterface string) BrowserControllerOption {
	return newFuncBrowserControllerOption(func(c *browserControllerOptions) {
		c.listenInterface = listenInterface
	})
}

func WithListenPort(port int) BrowserControllerOption {
	return newFuncBrowserControllerOption(func(c *browserControllerOptions) {
		c.listenPort = port
	})
}

func WithFrontierConn(conn *serviceconnections.FrontierConn) BrowserControllerOption {
	return newFuncBrowserControllerOption(func(c *browserControllerOptions) {
		c.frontierConn = conn
	})
}

func WithContentWriterConn(conn *serviceconnections.ContentWriterConn) BrowserControllerOption {
	return newFuncBrowserControllerOption(func(c *browserControllerOptions) {
		c.contentWriterConn = conn
	})
}

func WithRobotsEvaluatorConn(conn *serviceconnections.RobotsEvaluatorConn) BrowserControllerOption {
	return newFuncBrowserControllerOption(func(c *browserControllerOptions) {
		c.robotsEvaluatorConn = conn
	})
}

func WithSessionOptions(opts ...session.SessionOption) BrowserControllerOption {
	return newFuncBrowserControllerOption(func(c *browserControllerOptions) {
		c.sessionOpts = opts
	})
}

func WithMaxConcurrentSessions(maxSessions int) BrowserControllerOption {
	return newFuncBrowserControllerOption(func(c *browserControllerOptions) {
		c.maxSessions = maxSessions
	})
}

func WithCloseTimeout(d time.Duration) BrowserControllerOption {
	return newFuncBrowserControllerOption(func(c *browserControllerOptions) {
		c.closeTimeout = d
	})
}
