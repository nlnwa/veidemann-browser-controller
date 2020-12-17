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

package server

import (
	"context"
	"fmt"
	"github.com/nlnwa/veidemann-api/go/browsercontroller/v1"
	"reflect"
	"testing"
	"time"
)

func TestRecv(t *testing.T) {
	var testErr = fmt.Errorf("error")
	var testReq = &browsercontroller.DoRequest{}

	type args struct {
		ctx context.Context
		fn  func() (*browsercontroller.DoRequest, error)
	}
	tests := []struct {
		name    string
		args    args
		want    *browsercontroller.DoRequest
		wantErr error
	}{
		{
			"success",
			args{
				context.Background(),
				func() (*browsercontroller.DoRequest, error) {
					return testReq, nil
				},
			},
			testReq,
			nil,
		},
		{
			"error",
			args{
				context.Background(),
				func() (*browsercontroller.DoRequest, error) {
					return nil, testErr
				},
			},
			nil,
			testErr,
		},
		{
			"timeout",
			args{
				func() context.Context {
					ctx, _ := context.WithTimeout(context.Background(), 100*time.Millisecond)
					return ctx
				}(),
				func() (*browsercontroller.DoRequest, error) {
					time.Sleep(200 * time.Millisecond)
					return testReq, nil
				},
			},
			nil,
			context.DeadlineExceeded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Recv(tt.args.ctx, tt.args.fn)
			if err != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Run() got = %v, want %v", got, tt.want)
			}
		})
	}
}
