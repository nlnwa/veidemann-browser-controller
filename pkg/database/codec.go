package database

import (
	"encoding/json"
	"fmt"
	configV1 "github.com/nlnwa/veidemann-api-go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api-go/frontier/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gopkg.in/rethinkdb/rethinkdb-go.v6/encoding"
	"reflect"
	"time"
)

var decodeConfigObject = func(encoded interface{}, value reflect.Value) error {
	b, err := json.Marshal(encoded)
	if err != nil {
		return fmt.Errorf("error decoding ConfigObject: %w", err)
	}

	var co configV1.ConfigObject
	err = protojson.Unmarshal(b, &co)
	if err != nil {
		return fmt.Errorf("error decoding ConfigObject: %w", err)
	}

	value.Set(reflect.ValueOf(co))
	return nil
}

var encodeProtoMessage = func(value interface{}) (i interface{}, err error) {
	b, err := protojson.Marshal(value.(proto.Message))
	if err != nil {
		return nil, fmt.Errorf("error decoding ConfigObject: %w", err)
	}

	var m map[string]interface{}
	err = json.Unmarshal(b, &m)
	if err != nil {
		return nil, fmt.Errorf("error encoding proto message: %w", err)
	}
	return encoding.Encode(m)
}

func init() {
	encoding.SetTypeEncoding(
		reflect.TypeOf(&configV1.ConfigObject{}),
		encodeProtoMessage,
		decodeConfigObject,
	)
	encoding.SetTypeEncoding(
		reflect.TypeOf(&frontierV1.CrawlLog{}),
		encodeProtoMessage,
		nil,
	)
	encoding.SetTypeEncoding(
		reflect.TypeOf(&frontierV1.PageLog{}),
		encodeProtoMessage,
		nil,
	)
	encoding.SetTypeEncoding(
		reflect.TypeOf(map[string]interface{}{}),
		func(value interface{}) (i interface{}, err error) {
			m := value.(map[string]interface{})
			for k, v := range m {
				switch t := v.(type) {
				case string:
					// Try to parse string as date
					if ti, err := time.Parse(time.RFC3339Nano, t); err == nil {
						m[k] = ti
					} else {
						if m[k], err = encoding.Encode(v); err != nil {
							return nil, err
						}
					}
				default:
					if m[k], err = encoding.Encode(v); err != nil {
						return nil, err
					}
				}
			}
			return value, nil
		},
		func(encoded interface{}, value reflect.Value) error {
			value.Set(reflect.ValueOf(encoded))
			return nil
		},
	)
}
