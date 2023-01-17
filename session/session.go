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
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/security"
	"github.com/chromedp/cdproto/serviceworker"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/device"
	"github.com/google/uuid"
	configV1 "github.com/nlnwa/veidemann-api/go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api/go/frontier/v1"
	logV1 "github.com/nlnwa/veidemann-api/go/log/v1"
	"github.com/nlnwa/veidemann-browser-controller/database"
	"github.com/nlnwa/veidemann-browser-controller/errors"
	"github.com/nlnwa/veidemann-browser-controller/frontier"
	"github.com/nlnwa/veidemann-browser-controller/logwriter"
	"github.com/nlnwa/veidemann-browser-controller/requests"
	"github.com/nlnwa/veidemann-browser-controller/screenshotwriter"
	"github.com/nlnwa/veidemann-browser-controller/syncx"
	"github.com/nlnwa/whatwg-url/url"
	"github.com/opentracing/opentracing-go"
	tracelog "github.com/opentracing/opentracing-go/log"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Session struct {
	Id                int
	ctx               context.Context
	browserHost       string
	browserPort       int
	browserTimeout    int
	proxyHost         string
	proxyPort         int
	browserWsEndpoint string
	workspaceEndpoint string
	UserAgent         string
	browserVersion    string
	Requests          requests.RequestRegistry
	currentLoading    int32
	frameWg           *syncx.WaitGroup
	loadCancel        func()
	netActivityTimer  *syncx.CompletionTimer
	timer             *syncx.CompletionTimer
	RequestedUrl      *frontierV1.QueuedUri
	CrawlConfig       *configV1.CrawlConfig
	browserConfig     *configV1.BrowserConfig
	PolitenessConfig  *configV1.ConfigObject
	configCache       database.ConfigCache
	screenShotWriter  screenshotwriter.ScreenshotWriter
	logWriter         logwriter.LogWriter
	scripts           *sessionScripts
	logger            zerolog.Logger
}

func newDefaultSession(opts ...Option) *Session {
	s := &Session{
		browserHost:    "localhost",
		browserPort:    3000,
		browserTimeout: 500 * 1000,
		proxyPort:      3000,
	}
	for _, opt := range opts {
		opt.apply(s)
	}
	return s
}

func New(sessionId int, opts ...Option) (*Session, error) {
	s := newDefaultSession(opts...)
	s.Id = sessionId
	s.logger = zlog.Logger.With().Int("session", sessionId).Logger()

	ws, err := url.Parse("ws://" + s.browserHost + ":" + strconv.Itoa(s.browserPort))
	if err != nil {
		return nil, err
	}

	// Browserless lets you control how chrome is started via query string parameters
	// See https://www.browserless.io/docs/chrome-flags
	query := ws.SearchParams()
	if s.proxyHost != "" {
		proxy := "http://" + s.proxyHost + ":" + strconv.Itoa(s.proxyPort+s.Id)
		query.Set("--proxy-server", proxy)
	}
	query.Append("--ignore-certificate-errors", "")
	query.Append("headless", "true")
	query.Append("timeout", strconv.Itoa(s.browserTimeout))
	query.Append("trackingId", strconv.Itoa(s.Id))

	s.browserWsEndpoint = ws.String()

	s.logger.Debug().Str("wsURL", s.browserWsEndpoint).Msg("")

	work, err := url.Parse("http://" + s.browserHost + ":" + strconv.Itoa(s.browserPort) + "/workspace")
	if err != nil {
		return nil, err
	}
	s.workspaceEndpoint = work.String()

	return s, nil
}

func newDirectSession(uri, crawlExecutionId, jobExecutionId string, opts ...Option) (*Session, error) {
	sess := newDefaultSession(opts...)
	sess.Id = 0

	sess.logger = zlog.Logger.With().
		Str("uri", uri).
		Str("eid", crawlExecutionId).
		Str("jid", jobExecutionId).
		Int("session", sess.Id).
		Str("userAgent", sess.UserAgent).
		Logger()

	QUri := &frontierV1.QueuedUri{
		Uri:            uri,
		ExecutionId:    crawlExecutionId,
		JobExecutionId: jobExecutionId,
	}
	sess.RequestedUrl = QUri

	return sess, nil
}

func (sess *Session) Notify(reqId string) error {
	log := sess.logger

	if reqId == "" {
		log.Warn().Msg("Received notify with empty request ID")
	}
	select {
	case <-sess.ctx.Done():
		return status.Errorf(codes.Canceled, "Session is canceled")
	default:
		if sess.netActivityTimer != nil {
			sess.netActivityTimer.Notify()
		}
		if sess.timer != nil {
			sess.timer.Notify()
		}
		return nil
	}
}

func (sess *Session) Context() context.Context {
	return sess.ctx
}

func (sess *Session) Fetch(ctx context.Context, phs *frontierV1.PageHarvestSpec) (result *frontier.RenderResult, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "fetch")
	defer span.Finish()

	span.SetTag("eid", phs.GetQueuedUri().GetExecutionId()).
		SetTag("jeid", phs.GetQueuedUri().GetJobExecutionId()).
		LogFields(
			tracelog.String("uri", phs.GetQueuedUri().GetUri()),
			tracelog.String("seed", phs.GetQueuedUri().GetSeedUri()),
		)
	sess.logger = sess.logger.With().
		Str("uri", phs.GetQueuedUri().GetUri()).
		Str("eid", phs.GetQueuedUri().GetExecutionId()).
		Logger()
	log := sess.logger

	// Ensure that bugs in implementation is logged and handled
	defer func() {
		if r := recover(); r != nil {
			var fetchError errors.FetchError
			switch v := r.(type) {
			case errors.FetchError:
				fetchError = v
			case error:
				fetchError = errors.New(-5, "Runtime error", v.Error())
			default:
				fetchError = errors.New(-5, "Runtime error", fmt.Sprintf("%s", v))
			}
			// Add stacktrace to error
			fetchError.CommonsError().Detail += "\n" + string(debug.Stack())
			err = fetchError
		}
	}()

	log.Debug().Msg("Start fetch")
	sess.RequestedUrl = phs.GetQueuedUri()
	sess.CrawlConfig = phs.GetCrawlConfig().GetCrawlConfig()

	bConf, err := sess.configCache.GetConfigObject(ctx, sess.CrawlConfig.BrowserConfigRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get browser config: %v", err)
	}
	sess.browserConfig = bConf.GetBrowserConfig()

	sess.UserAgent = sess.browserConfig.UserAgent

	sess.PolitenessConfig, err = sess.configCache.GetConfigObject(ctx, sess.CrawlConfig.PolitenessRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get politeness config: %v", err)
	}

	maxTotalTime := time.Duration(sess.browserConfig.PageLoadTimeoutMs) * time.Millisecond
	maxIdleTime := time.Duration(sess.browserConfig.MaxInactivityTimeMs) * time.Millisecond

	if scripts, err := sess.loadScripts(ctx); err != nil {
		return nil, fmt.Errorf("failed to load scripts: %w", err)
	} else {
		sess.scripts = scripts
	}

	allocatorContext, allocatorCancel := chromedp.NewRemoteAllocator(ctx, sess.browserWsEndpoint, chromedp.NoModifyURL)
	defer allocatorCancel()
	defer sess.cleanWorkspace()

	// create context
	cdpCtx, cdpCancel := chromedp.NewContext(allocatorContext)
	defer cdpCancel()
	sess.ctx = cdpCtx

	var loadCtx context.Context
	loadCtx, sess.loadCancel = context.WithTimeout(sess.ctx, maxTotalTime)
	defer sess.loadCancel()

	sess.frameWg = syncx.NewWaitGroup(loadCtx)
	sess.Requests = requests.NewRegistry(sess.frameWg)

	sess.initListeners(cdpCtx)

	sess.netActivityTimer = syncx.NewCompletionTimer(1*time.Second, maxTotalTime, nil)
	sess.timer = syncx.NewCompletionTimer(maxIdleTime, maxTotalTime, sess.Requests.MatchCrawlLogs)

	// run task list
	if err := chromedp.Run(sess.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var userAgent string
			_, sess.browserVersion, _, userAgent, _, err = browser.GetVersion().Do(ctx)
			if sess.UserAgent == "" {
				sess.UserAgent = strings.ReplaceAll(userAgent, "HeadlessChrome", "Chrome")
			}
			return err
		}),
		security.SetIgnoreCertificateErrors(true),
		network.SetCacheDisabled(true),
		serviceworker.Enable(),
		chromedp.Emulate(&device.Info{
			Name:      "Desktop",
			UserAgent: sess.UserAgent,
			Width:     int64(sess.browserConfig.WindowWidth),
			Height:    int64(sess.browserConfig.WindowHeight),
			Scale:     1,
			Landscape: false,
			Mobile:    false,
			Touch:     false,
		}),
		fetch.Enable(),
		network.Enable(),
		page.Enable(),
		network.SetCookies(sess.getCookieParams(sess.RequestedUrl)),
		runtime.Enable(),
		target.SetAutoAttach(true, true).WithFlatten(true),
	); err != nil {
		return nil, fmt.Errorf("failed initializing browser: %w", err)
	}

	// Navigate
	fetchStart := time.Now()
	if err := chromedp.Run(loadCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, errorText, err := page.Navigate(sess.RequestedUrl.Uri).WithTransitionType(page.TransitionTypeOther).Do(ctx)
			if err != nil {
				return err
			}
			if errorText != "" {
				return fmt.Errorf("page load error %s", errorText)
			}
			return err
		}),
	); err != nil {
		if err == context.Canceled && sess.Requests.InitialRequest().FromCache {
			return nil, errors.New(-4100, "Already seen", "Initial request was found in cache. Url: "+sess.RequestedUrl.Uri)
		} else if err == context.DeadlineExceeded {
			return nil, errors.New(-5004, "Runtime exceeded", "Pageload timed out. Url: "+sess.RequestedUrl.Uri)
		} else {
			return nil, fmt.Errorf("failed navigation: %w", err)
		}
	}

	log.Debug().Msg("Wait for completion")
	// Give scripts a chance to start by waiting for network activity to slow down
	_ = sess.netActivityTimer.WaitForCompletion()
	sess.netActivityTimer.Reset()

	log.Debug().
		Str("phase", configV1.BrowserScript_ON_NEW_DOCUMENT.String()).Msg("Execute scripts")
	if err := sess.executeScripts(loadCtx, configV1.BrowserScript_ON_NEW_DOCUMENT); err != nil {
		log.Warn().
			Str("phase", configV1.BrowserScript_ON_NEW_DOCUMENT.String()).
			Err(err).Msg("Failed to execute scripts")
		return nil, fmt.Errorf("failed executing scripts in %v phase: %w", configV1.BrowserScript_ON_NEW_DOCUMENT, err)
	}
	log.Debug().
		Str("phase", configV1.BrowserScript_ON_LOAD.String()).Msg("Execute scripts")
	if err := sess.executeScripts(loadCtx, configV1.BrowserScript_ON_LOAD); err != nil {
		log.Warn().
			Str("phase", configV1.BrowserScript_ON_LOAD.String()).
			Err(err).Msg("Failed to execute scripts")
	}

	log.Debug().Msg("Wait for frames to finish loading")
	// Wait for frames to finish loading
	err = sess.frameWg.Wait()
	switch err {
	case syncx.Cancelled:
		if sess.Requests.InitialRequest().FromCache {
			return nil, errors.New(-4100, "Already seen", "Initial request was found in cache. Url: "+sess.RequestedUrl.Uri)
		}
	case syncx.ExceededMaxTime:
		if sess.Requests.InitialRequest() == nil || sess.Requests.InitialRequest().CrawlLog == nil {
			return nil, errors.New(-5004, "Runtime exceeded", "Pageload timed out. Url: "+sess.RequestedUrl.Uri)
		}
	}

	log.Debug().Msg("Wait for completion")

	// Wait for all outstanding requests to receive a response
	err = sess.timer.WaitForCompletion()
	if err != nil {
		log.Warn().Err(err).Msg("Fetch timed out")
	}

	fetchDuration := time.Since(fetchStart)

	sess.Requests.FinalizeResponses(sess.RequestedUrl)

	if sess.CrawlConfig.GetExtra().CreateScreenshot {
		sess.saveScreenshot()
	}
	outlinks := sess.extractOutlinks()
	log.Debug().Msgf("Found %d outlinks.", len(outlinks))
	cookies := sess.extractCookies()

	err = chromedp.Cancel(cdpCtx)
	if err != nil {
		log.Warn().Err(err).Msg("Close browser")
	}

	var crawlLogCount int32
	var bytesDownloaded int64
	var resources []*logV1.PageLog_Resource
	var crawlLogs []*logV1.CrawlLog

	sess.Requests.Walk(func(r *requests.Request) {
		if r.CrawlLog.GetWarcId() != "" {
			crawlLogs = append(crawlLogs, r.CrawlLog)
			crawlLogCount++
			bytesDownloaded += r.CrawlLog.Size
		} else {
			log.Trace().Msgf("Skipping write of %v %v %v, From cache %v, Has CrawlLog: %v", r.RequestId, r.Method, r.Url, r.FromCache, r.CrawlLog != nil)
		}

		if r.CrawlLog != nil {
			resource := &logV1.PageLog_Resource{
				Uri:           r.Url,
				FromCache:     r.FromCache,
				Renderable:    false,
				ResourceType:  r.ResourceType,
				ContentType:   r.CrawlLog.ContentType,
				StatusCode:    r.CrawlLog.StatusCode,
				DiscoveryPath: r.CrawlLog.DiscoveryPath,
				WarcId:        r.CrawlLog.WarcId,
				Referrer:      r.Referrer,
				Error:         r.CrawlLog.Error,
				Method:        r.Method,
			}
			resources = append(resources, resource)
		} else if !r.FromCache {
			log.Warn().Msgf("No crawllog for resource. Skipping %v %v %v. Got new: %v, Got complete %v", r.RequestId, r.Method, r.Url, r.GotNew, r.GotComplete)
		}
	})
	if err := sess.logWriter.WriteCrawlLogs(ctx, crawlLogs); err != nil {
		log.Error().Err(err).Msg("Writing crawl logs")
	} else {
		log.Debug().Msgf("Wrote %d crawlLogs", len(crawlLogs))
	}

	if sess.Requests.InitialRequest() != nil && sess.Requests.InitialRequest().CrawlLog != nil {
		warcId := sess.Requests.InitialRequest().CrawlLog.WarcId
		if warcId == "" {
			warcId = uuid.New().String()
		}
		pageLog := &logV1.PageLog{
			WarcId:              warcId,
			Uri:                 sess.RequestedUrl.Uri,
			ExecutionId:         sess.RequestedUrl.ExecutionId,
			Referrer:            sess.Requests.InitialRequest().Referrer,
			JobExecutionId:      sess.RequestedUrl.JobExecutionId,
			CollectionFinalName: sess.Requests.InitialRequest().CrawlLog.CollectionFinalName,
			Method:              sess.Requests.InitialRequest().Method,
			Resource:            resources,
			Outlink:             outlinks,
		}
		if err := sess.logWriter.WritePageLog(ctx, pageLog); err != nil {
			log.Error().Err(err).Msg("Error writing pageLog")
		} else {
			log.Debug().Msg("Pagelog written")
		}
	} else {
		return nil, fmt.Errorf("missing initial request: %w", err)
	}

	qUris := make([]*frontierV1.QueuedUri, len(outlinks))
	for i, uri := range outlinks {
		qUris[i] = &frontierV1.QueuedUri{
			ExecutionId:         sess.RequestedUrl.ExecutionId,
			DiscoveredTimeStamp: timestamppb.Now(),
			Uri:                 uri,
			DiscoveryPath:       sess.Requests.RootRequest().CrawlLog.DiscoveryPath + "L",
			Referrer:            sess.Requests.RootRequest().Url,
			Cookies:             cookies,
			JobExecutionId:      sess.RequestedUrl.JobExecutionId,
		}
	}
	result = &frontier.RenderResult{
		BytesDownloaded: bytesDownloaded,
		UriCount:        crawlLogCount,
		Outlinks:        qUris,
		Error:           sess.Requests.InitialRequest().CrawlLog.Error,
		PageFetchTimeMs: fetchDuration.Milliseconds(),
	}

	log.Debug().Msg("Fetch done")
	return result, nil
}

// cleanWorkspace removes downloaded resources in browser container
func (sess *Session) cleanWorkspace() {
	log := sess.logger

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r, err := http.NewRequestWithContext(ctx, "DELETE", sess.workspaceEndpoint+"/"+strconv.Itoa(sess.Id), nil)
	if err != nil {
		log.Warn().Err(err).Msg("Error creating request for cleaning up workspace")
	}
	if resp, err := http.DefaultClient.Do(r); err != nil {
		log.Warn().Err(err).Msg("Error cleaning up workspace")
	} else {
		_ = resp.Body.Close()
	}
}

func (sess *Session) getCookieParams(uri *frontierV1.QueuedUri) []*network.CookieParam {
	log := sess.logger

	log.Debug().Msgf("Restoring %v browser cookies", len(uri.GetCookies()))
	cookies := make([]*network.CookieParam, len(uri.GetCookies()))
	for i, c := range uri.GetCookies() {
		expSec, expNsec := math.Modf(c.Expires)
		expires := cdp.TimeSinceEpoch(time.Unix(int64(expSec), int64(expNsec*(1e9))))

		cookies[i] = &network.CookieParam{
			Name:     c.Name,
			Value:    c.Value,
			URL:      uri.Uri,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HttpOnly,
			SameSite: network.CookieSameSite(c.SameSite),
			Expires:  &expires,
		}
	}
	return cookies
}

func (sess *Session) extractCookies() []*frontierV1.Cookie {
	log := sess.logger
	var result []*frontierV1.Cookie

	if err := chromedp.Run(sess.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := network.GetCookies().Do(ctx)
			if err != nil {
				return err
			}

			result = make([]*frontierV1.Cookie, len(cookies))
			for i, c := range cookies {
				result[i] = &frontierV1.Cookie{
					Name:     c.Name,
					Value:    c.Value,
					Domain:   c.Domain,
					Path:     c.Path,
					Expires:  c.Expires,
					Size:     int32(c.Size),
					HttpOnly: c.HTTPOnly,
					Secure:   c.Secure,
					Session:  c.Session,
					SameSite: c.SameSite.String(),
				}
			}
			return nil
		}),
	); err != nil {
		log.Error().Err(err).Msg("Could not extract cookies")
	}

	return result
}

func (sess *Session) saveScreenshot() {
	log := sess.logger

	span, ctx := opentracing.StartSpanFromContext(sess.ctx, "screenshot")
	defer span.Finish()
	// Skip screenshot of pages loaded from cache
	if sess.Requests.RootRequest().FromCache {
		log.Debug().Str("resourceType", sess.Requests.RootRequest().ResourceType).Msgf("Skipping screenshot: from cache")
		return
	}
	// Check if page is renderable
	if sess.Requests.RootRequest().ResourceType != "Document" && sess.Requests.RootRequest().ResourceType != "Image" {
		log.Debug().Str("resourceType", sess.Requests.RootRequest().ResourceType).Msgf("Skipping screenshot: not renderable")
		return
	}
	// Check if CrawlLog is present for root request
	if sess.Requests.RootRequest().CrawlLog == nil {
		log.Debug().Str("resourceType", sess.Requests.RootRequest().ResourceType).Msgf("Skipping screenshot: missing crawlLog")
		return
	}
	// Check if CrawlLog has WarcId
	if sess.Requests.RootRequest().CrawlLog.WarcId == "" {
		log.Debug().Str("resourceType", sess.Requests.RootRequest().ResourceType).Msgf("Skipping screenshot: crawlLog has empty warcId")
		return
	}
	var data []byte
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) (err error) {
			data, err = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatPng).Do(ctx)
			return
		}),
	)
	if err != nil {
		log.Error().Err(err).Msg("Error capturing screenshot")
		return
	}
	metadata := screenshotwriter.Metadata{
		CrawlConfig:    sess.CrawlConfig,
		CrawlLog:       sess.Requests.RootRequest().CrawlLog,
		BrowserConfig:  sess.browserConfig,
		BrowserVersion: sess.browserVersion,
	}
	if err = sess.screenShotWriter.Write(ctx, data, metadata); err != nil {
		log.Error().Err(err).Msg("Error writing screenshot")
		return
	}
}

func (sess *Session) extractOutlinks() []string {
	var extractedUrls []string

	for _, s := range sess.scripts.Get(configV1.BrowserScript_EXTRACT_OUTLINKS) {
		log := sess.logger.With().
			Str("scriptType", configV1.BrowserScript_EXTRACT_OUTLINKS.String()).
			Str("scriptId", s.GetId()).
			Logger()

		var res *runtime.RemoteObject
		var exceptionDetails *runtime.ExceptionDetails
		err := chromedp.Run(sess.ctx,
			chromedp.ActionFunc(func(ctx context.Context) (err error) {
				res, exceptionDetails, err = runtime.Evaluate(s.GetBrowserScript().GetScript()).
					WithReturnByValue(true).Do(ctx)
				return err
			}),
		)
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to evaluate script")
			continue
		}
		if exceptionDetails != nil {
			log.Warn().Err(exceptionDetails).Msgf("Exception during script evaluation")
			continue
		}
		if res.Value != nil {
			var links []string
			err := json.Unmarshal(res.Value, &links)
			if err != nil {
				log.Warn().Err(err).Msgf("Failed to unmarshal return value")
				continue
			}

			for _, link := range links {
				link = strings.TrimSpace(link)
				link = strings.Trim(link, "\"\\")
				if link != "" && link != sess.Requests.RootRequest().Url {
					extractedUrls = append(extractedUrls, link)
				}
			}
		}
	}

	return extractedUrls
}

func (sess *Session) AbortFetch() error {
	defer sess.loadCancel()
	defer sess.frameWg.Cancel()
	return chromedp.Run(sess.ctx, page.StopLoading())
}
