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

package testutil

import (
	"context"
	"github.com/nlnwa/veidemann-browser-controller/screenshotwriter"
)

// ScreenshotWriterMock is a helper structure for mocking screenshotwriter.ScreenshotWriter
type ScreenshotWriterMock struct {
	// optional function to call at connect time
	ConnectFunc func() error
	// optional function to call when RobotsEvaluator is closing
	CloseFunc func() error
	// optional function to call when Write is called on ScreenshotWriter. Write will always return nil if this
	// is not set
	WriteFunc func(data []byte, metadata screenshotwriter.Metadata) error
}

func (s *ScreenshotWriterMock) Connect() error {
	if s.ConnectFunc != nil {
		return s.ConnectFunc()
	}
	return nil
}

func (s *ScreenshotWriterMock) Close() error {
	if s.CloseFunc != nil {
		return s.CloseFunc()
	}
	return nil
}

func (s *ScreenshotWriterMock) Write(_ context.Context, data []byte, metadata screenshotwriter.Metadata) error {
	if s.WriteFunc != nil {
		return s.WriteFunc(data, metadata)
	}
	return nil
}
