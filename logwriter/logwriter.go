/*
 * Copyright 2021 National Library of Norway.
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

package logwriter

import (
	"context"
	logV1 "github.com/nlnwa/veidemann-api/go/log/v1"
	"github.com/nlnwa/veidemann-browser-controller/serviceconnections"
	"github.com/nlnwa/veidemann-log-service/pkg/logservice"
)

type LogWriter interface {
	serviceconnections.Connection
	WritePageLog(context.Context, *logV1.PageLog) error
	WriteCrawlLog(context.Context, *logV1.CrawlLog) error
	WriteCrawlLogs(context.Context, []*logV1.CrawlLog) error
}

type logWriter struct {
	*serviceconnections.ClientConn
	*logservice.LogWriter
}

func New(opts ...serviceconnections.ConnectionOption) LogWriter {
	return &logWriter{
		ClientConn: serviceconnections.NewClientConn("logWriter", opts...),
	}
}

func (l *logWriter) Connect() error {
	if err := l.ClientConn.Connect(); err != nil {
		return err
	}
	l.LogWriter = &logservice.LogWriter{
		LogClient: logV1.NewLogClient(l.ClientConn.Connection()),
	}
	return nil
}
