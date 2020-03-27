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

package controller

import (
	"context"
	"crypto/sha1"
	"fmt"
	configV1 "github.com/nlnwa/veidemann-api-go/config/v1"
	contentwriterV1 "github.com/nlnwa/veidemann-api-go/contentwriter/v1"
	"github.com/nlnwa/veidemann-browser-controller/pkg/session"
	log "github.com/sirupsen/logrus"
)

func (bc *BrowserController) writeScreenshot(ctx context.Context, sess *session.Session, data []byte) error {
	stream, err := bc.opts.contentWriterConn.Client().Write(ctx)
	if err != nil {
		return fmt.Errorf("failed to open Frontier session: %w", err)
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
	cl := sess.Requests.RootRequest().CrawlLog
	browserConfig := sess.BrowserConfig

	screenshotMetaRecord := fmt.Sprintf(
		"browserVersion: %s\r\nwindowHeight: %d\r\nwindowWidth: %d\r\nuserAgent: %s\r\n",
		sess.BrowserVersion,
		browserConfig.GetWindowHeight(),
		browserConfig.GetWindowWidth(),
		browserConfig.GetUserAgent())
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
		WarcConcurrentTo:  []string{cl.WarcId},
	}
	screenshotMetaRecordMeta := &contentwriterV1.WriteRequestMeta_RecordMeta{
		RecordNum:         1,
		Type:              contentwriterV1.RecordType_METADATA,
		RecordContentType: "application/warc-fields",
		BlockDigest:       screenshotMetaRecordDigest,
		Size:              int64(len(screenshotMetaRecord)),
		SubCollection:     configV1.Collection_SCREENSHOT,
		WarcConcurrentTo:  []string{cl.WarcId},
	}

	ip := cl.IpAddress
	if ip == "" {
		log.Errorf("Missing IP address for screenshot, using 127.0.0.1")
		ip = "127.0.0.1"
	}

	meta := &contentwriterV1.WriteRequest_Meta{Meta: &contentwriterV1.WriteRequestMeta{
		ExecutionId: cl.ExecutionId,
		TargetUri:   cl.RequestedUri,
		RecordMeta: map[int32]*contentwriterV1.WriteRequestMeta_RecordMeta{
			0: screenshotRecordMeta,
			1: screenshotMetaRecordMeta,
		},
		FetchTimeStamp: cl.FetchTimeStamp,
		IpAddress:      ip,
		CollectionRef:  sess.CrawlConfig.CollectionRef,
	}}
	if err = stream.Send(&contentwriterV1.WriteRequest{Value: meta}); err != nil {
		return err
	}
	if response, err := stream.CloseAndRecv(); err != nil {
		return err
	} else {
		log.Debugf("Screenshot written: %v\n", response.GetMeta().RecordMeta)
	}
	return nil
}
