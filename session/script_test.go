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

package session

import (
	"testing"
)

func TestMatch(t *testing.T) {
	type sample struct {
		uri  string
		want bool
	}
	type test struct {
		pattern string
		samples []sample
	}
	tests := []test{
		{
			"^https://example[.]com/$",
			[]sample{
				{"https://example.com/", true},
				{"https://example.com/bad", false},
			},
		},
		{
			"^https://example[.]com/",
			[]sample{
				{"https://example.com/", true},
				{"https://example.com/bad", true},
			},
		},
	}

	for _, test := range tests {
		for _, sample := range test.samples {
			regexps := []string{test.pattern}
			t.Run(sample.uri, func(t *testing.T) {
				got := match(regexps, sample.uri)
				if got != sample.want {
					t.Errorf("Got %t; want %t", got, sample.want)
				}
			})
		}
	}
}

