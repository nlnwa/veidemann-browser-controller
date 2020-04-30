package server

import (
	"context"
	"fmt"
	"github.com/nlnwa/veidemann-api-go/browsercontroller/v1"
	"reflect"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
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
			got, err := Run(tt.args.ctx, tt.args.fn)
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
