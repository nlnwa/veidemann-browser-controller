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
	robotsevaluatorV1 "github.com/nlnwa/veidemann-api/go/robotsevaluator/v1"
)

// RobotsEvaluatorMock is a helper structure for mocking robotsevaluator.RobotsEvaluator
type RobotsEvaluatorMock struct {
	// optional function to call at connect time
	ConnectFunc func() error
	// optional function to call when RobotsEvaluator is closing
	CloseFunc func() error
	// optional function to call when IsAllowed is called on RobotsEvaluator. IsAllowed will always return true if this
	// is not set
	IsAllowedFunc func(*robotsevaluatorV1.IsAllowedRequest) bool
}

func (r *RobotsEvaluatorMock) Connect() error {
	if r.ConnectFunc != nil {
		return r.ConnectFunc()
	}
	return nil
}

func (r *RobotsEvaluatorMock) Close() error {
	if r.CloseFunc != nil {
		return r.CloseFunc()
	}
	return nil
}

func (r *RobotsEvaluatorMock) IsAllowed(_ context.Context, request *robotsevaluatorV1.IsAllowedRequest) bool {
	if r.IsAllowedFunc != nil {
		return r.IsAllowedFunc(request)
	}
	return true
}
