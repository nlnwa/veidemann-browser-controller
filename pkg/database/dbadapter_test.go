package database

import (
	configV1 "github.com/nlnwa/veidemann-api/go/config/v1"
	"github.com/nlnwa/veidemann-api/go/frontier/v1"
	"reflect"
	"testing"
	"time"
)

var (
	v1 = &configV1.ConfigObject{Kind: configV1.Kind_crawlJob, Id: "1", Meta: &configV1.Meta{Name: "1"}}
	v2 = &configV1.ConfigObject{Kind: configV1.Kind_crawlJob, Id: "2", Meta: &configV1.Meta{Name: "2"}}
	v3 = &configV1.ConfigObject{Kind: configV1.Kind_crawlJob, Id: "3", Meta: &configV1.Meta{Name: "3"}}
)

type dbConnMock struct {
	i int
}

func (d *dbConnMock) GetConfig(ref *configV1.ConfigRef) (*configV1.ConfigObject, error) {
	d.i++
	switch d.i {
	case 1:
		return v1, nil
	case 2:
		return v2, nil
	default:
		return v3, nil
	}
}

func (d *dbConnMock) Connect() error {
	panic("implement me")
}

func (d *dbConnMock) Close() error {
	panic("implement me")
}

func (d *dbConnMock) GetConfigsForSelector(kind configV1.Kind, label *configV1.Label) ([]*configV1.ConfigObject, error) {
	panic("implement me")
}

func (d *dbConnMock) WriteCrawlLog(crawlLog *frontier.CrawlLog) error {
	panic("implement me")
}

func (d *dbConnMock) WriteCrawlLogs(crawlLog []*frontier.CrawlLog) error {
	panic("implement me")
}

func (d *dbConnMock) WritePageLog(pageLog *frontier.PageLog) error {
	panic("implement me")
}

func Test_configCache_Get(t *testing.T) {
	tests := []struct {
		name       string
		sleep      time.Duration
		wantFirst  *configV1.ConfigObject
		wantSecond *configV1.ConfigObject
		wantErr    bool
	}{
		{"same", 10 * time.Millisecond, v1, v1, false},
		{"evicted", 110 * time.Millisecond, v1, v2, false},
	}
	for _, tt := range tests {
		//i := 0
		t.Run(tt.name, func(t *testing.T) {
			cc := NewDbAdapter(&dbConnMock{i: 0}, 100*time.Millisecond)
			ref := &configV1.ConfigRef{Kind: configV1.Kind_crawlJob, Id: "1"}

			gotFirst, err := cc.GetConfigObject(ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("1 GetConfigObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotFirst, tt.wantFirst) {
				t.Errorf("1 GetConfigObject() got = %v, want %v", gotFirst, tt.wantFirst)
			}

			time.Sleep(tt.sleep)

			gotSecond, err := cc.GetConfigObject(ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("2 GetConfigObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotSecond, tt.wantSecond) {
				t.Errorf("2 GetConfigObject() got = %v, want %v", gotSecond, tt.wantSecond)
			}
			if gotSecond != tt.wantSecond {
				t.Errorf("2 GetConfigObject() got = %v, want %v", gotSecond, tt.wantSecond)
			}
		})
	}
}
