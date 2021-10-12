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

package screenshotwriter

import (
	"context"
	"crypto/sha1"
	"fmt"
	configV1 "github.com/nlnwa/veidemann-api/go/config/v1"
	contentwriterV1 "github.com/nlnwa/veidemann-api/go/contentwriter/v1"
	logV1 "github.com/nlnwa/veidemann-api/go/log/v1"
	"github.com/nlnwa/veidemann-browser-controller/serviceconnections"
	"github.com/rs/zerolog/log"
)

type Metadata struct {
	CrawlConfig    *configV1.CrawlConfig
	CrawlLog       *logV1.CrawlLog
	BrowserConfig  *configV1.BrowserConfig
	BrowserVersion string
}

type ScreenshotWriter interface {
	serviceconnections.Connection
	Write(context.Context, []byte, Metadata) error
}

type screenshotWriter struct {
	*serviceconnections.ClientConn
	contentwriterV1.ContentWriterClient
}

func New(opts ...serviceconnections.ConnectionOption) ScreenshotWriter {
	return &screenshotWriter{
		ClientConn: serviceconnections.NewClientConn("ContentWriter", opts...),
	}
}

func (s *screenshotWriter) Connect() error {
	if err := s.ClientConn.Connect(); err != nil {
		return err
	} else {
		s.ContentWriterClient = contentwriterV1.NewContentWriterClient(s.ClientConn.Connection())
		return nil
	}
}

func (s *screenshotWriter) Write(ctx context.Context, data []byte, metadata Metadata) error {
	stream, err := s.ContentWriterClient.Write(ctx)
	if err != nil {
		return fmt.Errorf("failed to open ContentWriter session: %w", err)
	}

	h := sha1.New()
	h.Write(data)
	digest := fmt.Sprintf("sha1:%x", h.Sum(nil))

	d := &contentwriterV1.WriteRequest_Payload{Payload: &contentwriterV1.Data{
		RecordNum: 0,
		Data:      data,
	}}
	if err = stream.Send(&contentwriterV1.WriteRequest{Value: d}); err != nil {
		return err
	}

	screenshotMetaRecord := fmt.Sprintf(
		"browserVersion: %s\r\nwindowHeight: %d\r\nwindowWidth: %d\r\nuserAgent: %s\r\n",
		metadata.BrowserVersion,
		metadata.BrowserConfig.GetWindowHeight(),
		metadata.BrowserConfig.GetWindowWidth(),
		metadata.BrowserConfig.GetUserAgent())
	d = &contentwriterV1.WriteRequest_Payload{Payload: &contentwriterV1.Data{
		RecordNum: 1,
		Data:      []byte(screenshotMetaRecord),
	}}
	if err = stream.Send(&contentwriterV1.WriteRequest{Value: d}); err != nil {
		return err
	}

	h.Reset()
	h.Write([]byte(screenshotMetaRecord))
	screenshotMetaRecordDigest := fmt.Sprintf("sha1:%x", h.Sum(nil))

	screenshotRecordMeta := &contentwriterV1.WriteRequestMeta_RecordMeta{
		RecordNum:         0,
		Type:              contentwriterV1.RecordType_RESOURCE,
		RecordContentType: "image/png",
		BlockDigest:       digest,
		Size:              int64(len(data)),
		SubCollection:     configV1.Collection_SCREENSHOT,
		WarcConcurrentTo:  []string{metadata.CrawlLog.WarcId},
	}
	screenshotMetaRecordMeta := &contentwriterV1.WriteRequestMeta_RecordMeta{
		RecordNum:         1,
		Type:              contentwriterV1.RecordType_METADATA,
		RecordContentType: "application/warc-fields",
		BlockDigest:       screenshotMetaRecordDigest,
		Size:              int64(len(screenshotMetaRecord)),
		SubCollection:     configV1.Collection_SCREENSHOT,
		WarcConcurrentTo:  []string{metadata.CrawlLog.WarcId},
	}

	ip := metadata.CrawlLog.IpAddress
	if ip == "" {
		log.Warn().Msg("Missing IP address for screenshot, using 127.0.0.1")
		ip = "127.0.0.1"
	}

	meta := &contentwriterV1.WriteRequest_Meta{Meta: &contentwriterV1.WriteRequestMeta{
		ExecutionId: metadata.CrawlLog.ExecutionId,
		TargetUri:   metadata.CrawlLog.RequestedUri,
		RecordMeta: map[int32]*contentwriterV1.WriteRequestMeta_RecordMeta{
			0: screenshotRecordMeta,
			1: screenshotMetaRecordMeta,
		},
		FetchTimeStamp: metadata.CrawlLog.FetchTimeStamp,
		IpAddress:      ip,
		CollectionRef:  metadata.CrawlConfig.GetCollectionRef(),
	}}
	if err = stream.Send(&contentwriterV1.WriteRequest{Value: meta}); err != nil {
		return err
	}
	if response, err := stream.CloseAndRecv(); err != nil {
		return err
	} else {
		log.Debug().
			Str("url", metadata.CrawlLog.GetRequestedUri()).
			Int32("width", metadata.BrowserConfig.GetWindowWidth()).
			Int32("height", metadata.BrowserConfig.GetWindowHeight()).
			Int("records", len(response.GetMeta().GetRecordMeta())).
			Msg("Screenshot written")
	}
	return nil
}
