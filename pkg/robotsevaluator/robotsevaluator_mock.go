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

package robotsevaluator

import (
	"context"
	robotsevaluatorV1 "github.com/nlnwa/veidemann-api-go/robotsevaluator/v1"
)

type robotsEvaluatorMock struct {
	isAllowed bool
}

func NewMock(isAllowed bool) RobotsEvaluator {
	return &robotsEvaluatorMock{isAllowed}
}

func (r *robotsEvaluatorMock) IsAllowed(_ context.Context, _ *robotsevaluatorV1.IsAllowedRequest) (bool, error) {
	return r.isAllowed, nil
}
