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
	"github.com/golang/protobuf/ptypes"
	"github.com/mailru/easyjson"
	configV1 "github.com/nlnwa/veidemann-api-go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api-go/frontier/v1"
	robotsevaluatorV1 "github.com/nlnwa/veidemann-api-go/robotsevaluator/v1"
	"github.com/nlnwa/veidemann-browser-controller/pkg/database"
	"github.com/nlnwa/veidemann-browser-controller/pkg/errors"
	"github.com/nlnwa/veidemann-browser-controller/pkg/requests"
	"github.com/nlnwa/veidemann-browser-controller/pkg/syncx"
	"github.com/nlnwa/whatwg-url/url"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"math"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

type ReturnValue struct {
	WaitForData bool            `json:"waitForData,omitempty"`
	Next        string          `json:"next,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
}

type Session struct {
	Id                int
	ctx               context.Context
	ecd               []*runtime.ExecutionContextDescription
	browserHost       string
	browserPort       int
	browserTimeout    int
	proxyHost         string
	proxyPort         int
	browserWsEndpoint string
	workspaceEndpoint string
	UserAgent         string
	BrowserVersion    string
	Requests          requests.RequestRegistry
	currentLoading    int32
	frameWg           *syncx.WaitGroup
	loadCancel        func()
	onLoadWg          *syncx.WaitGroup
	netActivityTimer  *syncx.CompletionTimer
	timer             *syncx.CompletionTimer
	RequestedUrl      *frontierV1.QueuedUri
	CrawlConfig       *configV1.CrawlConfig
	BrowserConfig     *configV1.BrowserConfig
	PolitenessConfig  *configV1.ConfigObject
	DbAdapter         *database.DbAdapter
	Fetch             func(QUri *frontierV1.QueuedUri, crawlConfig *configV1.ConfigObject) (*RenderResult, error)
	RobotsIsAllowed   func(ctx context.Context, request *robotsevaluatorV1.IsAllowedRequest) bool
	WriteScreenshot   func(ctx context.Context, sess *Session, data []byte) error

	// Temporary workaround until we have proper configuration
	scrollPages int
}

func New(sessionId int, opts ...SessionOption) (*Session, error) {
	s := defaultSessionOptions()
	s.Id = sessionId
	for _, opt := range opts {
		opt.apply(s)
	}

	ws, err := url.Parse("ws://" + s.browserHost + ":" + strconv.Itoa(s.browserPort))
	if err != nil {
		return nil, err
	}
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

	work, err := url.Parse("http://" + s.browserHost + ":" + strconv.Itoa(s.browserPort) + "/workspace")
	if err != nil {
		return nil, err
	}
	s.workspaceEndpoint = work.String()

	log.Debugf("New session. Id: %d, CDP endpoint: %v", s.Id, s.browserWsEndpoint)
	return s, nil
}

func newDirectSession(ctx context.Context, uri, crawlExecutionId, jobExecutionId string, opts ...SessionOption) (*Session, error) {
	sess := defaultSessionOptions()
	sess.Id = 0
	for _, opt := range opts {
		opt.apply(sess)
	}
	sess.ctx, _ = context.WithTimeout(ctx, 10*time.Second)

	QUri := &frontierV1.QueuedUri{
		Uri:            uri,
		ExecutionId:    crawlExecutionId,
		JobExecutionId: jobExecutionId,
	}
	log.WithField("eid", QUri.ExecutionId).Infof("Start fetch of %v", QUri.Uri)
	sess.RequestedUrl = QUri

	log.Tracef("New direct session. Id: %d, %v", sess.Id, sess.UserAgent)
	return sess, nil
}

func (sess *Session) Notify(reqId string) error {
	if reqId == "" {
		log.Warnf("Notify without request")
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

func (sess *Session) Done() <-chan struct{} {
	if sess.ctx != nil {
		return sess.ctx.Done()
	} else {
		return nil
	}
}

func (sess *Session) fetch(QUri *frontierV1.QueuedUri, crawlConf *configV1.ConfigObject) (result *RenderResult, err error) {
	// Ensure that bugs in implementation is logged and handled
	defer func() {
		if r := recover(); r != nil {
			var fetchError errors.FetchError
			switch v := r.(type) {
			case errors.FetchError:
				fetchError = v
			case error:
				fetchError = errors.New(-5, "Runtime error", v.Error())
			case *log.Entry:
				fetchError = errors.New(-5, "Runtime error", v.Message)
			default:
				fetchError = errors.New(-5, "Runtime error", fmt.Sprintf("%s", v))
			}
			log.WithField("eid", QUri.ExecutionId).Errorf("Panic while fetching %v: %s", QUri.Uri, fetchError.Error())

			// Add stacktrace to error
			fetchError.CommonsError().Detail += "\n" + string(debug.Stack())
			err = fetchError
		}
	}()

	log.WithField("eid", QUri.ExecutionId).Infof("Start fetch of %v", QUri.Uri)
	sess.RequestedUrl = QUri
	sess.CrawlConfig = crawlConf.GetCrawlConfig()

	bConf, err := sess.DbAdapter.GetConfigObject(sess.CrawlConfig.BrowserConfigRef)
	if err != nil {
		return nil, err
	}
	sess.BrowserConfig = bConf.GetBrowserConfig()

	sess.PolitenessConfig, err = sess.DbAdapter.GetConfigObject(sess.CrawlConfig.PolitenessRef)
	if err != nil {
		return nil, err
	}

	maxTotalTime := time.Duration(sess.BrowserConfig.PageLoadTimeoutMs) * time.Millisecond
	maxIdleTime := time.Duration(sess.BrowserConfig.MaxInactivityTimeMs) * time.Millisecond

	allocatorContext, allocatorCancel := chromedp.NewRemoteAllocator(context.Background(), sess.browserWsEndpoint)
	defer allocatorCancel()

	// create context
	ctx, cancel := chromedp.NewContext(allocatorContext)
	defer cancel()
	sess.ctx = ctx

	// ensure the first tab is created
	var userAgent string
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, sess.BrowserVersion, _, userAgent, _, err = browser.GetVersion().Do(ctx)
			return err
		}),
	); err != nil {
		return nil, err
	}

	var loadCtx context.Context
	loadCtx, sess.loadCancel = context.WithTimeout(sess.ctx, maxTotalTime)

	sess.onLoadWg = syncx.NewWaitGroup(loadCtx)
	sess.onLoadWg.Add(1)

	sess.frameWg = syncx.NewWaitGroup(sess.ctx)
	sess.Requests = requests.NewRegistry(sess.ctx, sess.frameWg)

	sess.initListeners()

	sess.netActivityTimer = syncx.NewCompletionTimer(1*time.Second, maxTotalTime, nil)
	sess.timer = syncx.NewCompletionTimer(maxIdleTime, maxTotalTime, sess.Requests.MatchCrawlLogs)

	sess.UserAgent = sess.BrowserConfig.UserAgent
	if sess.UserAgent == "" {
		sess.UserAgent = strings.ReplaceAll(userAgent, "HeadlessChrome", "Chrome")
	}

	deviceInfo := &device.Info{
		Name:      "Desktop",
		UserAgent: sess.UserAgent,
		Width:     int64(sess.BrowserConfig.WindowWidth),
		Height:    int64(sess.BrowserConfig.WindowHeight),
		Scale:     1,
		Landscape: false,
		Mobile:    false,
		Touch:     false,
	}

	// run task list
	if err := chromedp.Run(ctx,
		security.SetIgnoreCertificateErrors(true),
		network.SetCacheDisabled(true),
		serviceworker.Enable(),
		chromedp.Emulate(deviceInfo),
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
			_, _, _, err := page.Navigate(sess.RequestedUrl.Uri).WithTransitionType(page.TransitionTypeOther).Do(ctx)
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

	// wait for load event
	err = sess.onLoadWg.Wait()
	if err != nil {
		if err == context.DeadlineExceeded {
			return nil, errors.New(-5004, "Runtime exceeded", "Waiting for load event. Url: "+sess.RequestedUrl.Uri)
		} else {
			return nil, fmt.Errorf("waiting for load event: %w", err)
		}
	}
	// wait for activity to settle after load event
	_ = sess.netActivityTimer.WaitForCompletion()
	sess.netActivityTimer.Reset()

	sess.executeOnLoadScripts()

	// Give scripts a chance to start by waiting for network activity to slow down
	_ = sess.netActivityTimer.WaitForCompletion()

	// Wait for frames to finish loading
	err = sess.frameWg.Wait()
	if err == syncx.Cancelled && sess.Requests.InitialRequest().FromCache {
		return nil, errors.New(-4100, "Already seen", "Initial request was found in cache. Url: "+sess.RequestedUrl.Uri)
	} else if err == syncx.ExceededMaxTime && (sess.Requests.InitialRequest() == nil || sess.Requests.InitialRequest().CrawlLog == nil) {
		return nil, errors.New(-5004, "Runtime exceeded", "Pageload timed out. Url: "+sess.RequestedUrl.Uri)
	} else if err == syncx.IdleTimeout && (sess.Requests.InitialRequest() == nil || sess.Requests.InitialRequest().CrawlLog == nil) {
		return nil, errors.New(-4, "Http timeout", "Idle time out. Url: "+sess.RequestedUrl.Uri)
	}

	// Wait for all outstanding requests to receive a response
	err = sess.timer.WaitForCompletion()
	if err != nil {
		log.Warnf("Not complete: %v", err)
	}

	fetchDuration := time.Since(fetchStart)

	sess.Requests.FinalizeResponses(sess.RequestedUrl)

	var crawlLogCount int32
	var bytesDownloaded int64
	var resources []*frontierV1.PageLog_Resource

	sess.Requests.Walk(func(r *requests.Request) {
		if r.CrawlLog != nil && r.CrawlLog.WarcId != "" {
			if err := sess.DbAdapter.WriteCrawlLog(r.CrawlLog); err != nil {
				log.Errorf("error writing crawlLog: %v", err)
				return
			}
			crawlLogCount++
			bytesDownloaded += r.CrawlLog.Size
		} else {
			log.Tracef("Skipping write of %v %v %v, From cache %v, Has CrawlLog: %v", r.RequestId, r.Method, r.Url, r.FromCache, r.CrawlLog != nil)
		}

		if r.CrawlLog != nil {
			resource := &frontierV1.PageLog_Resource{
				Uri:           r.Url,
				FromCache:     r.FromCache,
				Renderable:    false,
				ResourceType:  r.ResourceType,
				MimeType:      r.CrawlLog.ContentType,
				StatusCode:    r.CrawlLog.StatusCode,
				DiscoveryPath: r.CrawlLog.DiscoveryPath,
				WarcId:        r.CrawlLog.WarcId,
				Referrer:      r.Referrer,
				Error:         r.CrawlLog.Error,
				Method:        r.Method,
			}
			resources = append(resources, resource)
		} else if !r.FromCache {
			log.Warnf("No crawllog for resource. Skipping %v %v %v. Got new: %v, Got complete %v", r.RequestId, r.Method, r.Url, r.GotNew, r.GotComplete)
		}
	})

	if sess.CrawlConfig.Extra.CreateScreenshot {
		sess.saveScreenshot()
	}

	qUris := sess.extractOutlinks()
	outlinks := make([]string, len(qUris))
	for _, outlink := range qUris {
		outlinks = append(outlinks, outlink.Uri)
	}

	err = chromedp.Cancel(ctx)
	if err != nil {
		log.Warnf("Failed closing browser: %v", err)
	}

	if sess.Requests.InitialRequest() != nil && sess.Requests.InitialRequest().CrawlLog != nil {
		pageLog := &frontierV1.PageLog{
			WarcId:              sess.Requests.InitialRequest().CrawlLog.WarcId,
			Uri:                 sess.RequestedUrl.Uri,
			ExecutionId:         sess.RequestedUrl.ExecutionId,
			Referrer:            sess.Requests.InitialRequest().Referrer,
			JobExecutionId:      sess.RequestedUrl.JobExecutionId,
			CollectionFinalName: sess.Requests.InitialRequest().CrawlLog.CollectionFinalName,
			Method:              sess.Requests.InitialRequest().Method,
			Resource:            resources,
			Outlink:             outlinks,
		}
		if err := sess.DbAdapter.WritePageLog(pageLog); err != nil {
			return nil, fmt.Errorf("error writing pageLog: %w", err)
		} else {
			log.WithField("uri", sess.RequestedUrl.Uri).Debugf("Pagelog written")
		}
	} else {
		return nil, fmt.Errorf("missing initial request: %w", err)
	}

	result = &RenderResult{
		BytesDownloaded: bytesDownloaded,
		UriCount:        crawlLogCount,
		Outlinks:        qUris,
		Error:           sess.Requests.InitialRequest().CrawlLog.Error,
		PageFetchTimeMs: fetchDuration.Milliseconds(),
	}

	sess.cleanWorkspace()
	log.Debugf("Fetch done: %v", QUri.Uri)
	return result, nil
}

func (sess *Session) callScript(eci runtime.ExecutionContextID, fn string, arguments json.RawMessage) (easyjson.RawMessage, error) {
	var res *runtime.RemoteObject
	var exceptionDetails *runtime.ExceptionDetails
	err := chromedp.Run(sess.ctx,
		chromedp.ActionFunc(func(ctx context.Context) (err error) {
			res, exceptionDetails, err = runtime.
				CallFunctionOn(fn).
				WithArguments([]*runtime.CallArgument{{Value: easyjson.RawMessage(arguments)}}).
				WithExecutionContextID(eci).
				WithReturnByValue(true).
				Do(ctx)
			return err
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", exceptionDetails, err)
	}
	return res.Value, nil
}

func (sess *Session) executeOnLoadScripts() {
	log.Debugf("%v", configV1.BrowserScript_ON_LOAD)

	seedAnnotations := sess.DbAdapter.GetSeedByUri(sess.RequestedUrl).GetMeta().GetAnnotation()

	undefinedScripts := make(map[string]string)
	undefinedScriptAnnotations := make(map[string][]*configV1.Annotation)
	for _, script := range sess.DbAdapter.GetScripts(sess.BrowserConfig, configV1.BrowserScript_UNDEFINED) {
		undefinedScripts[script.Meta.Name] = script.GetBrowserScript().GetScript()
		undefinedScriptAnnotations[script.Meta.Name] = script.GetMeta().GetAnnotation()
	}

	for _, script := range sess.DbAdapter.GetScripts(sess.BrowserConfig, configV1.BrowserScript_ON_LOAD) {
		scriptAnnotations := script.GetMeta().GetAnnotation()

		// merge annotations from seed and script
		annotations := make(map[string]string)
		for _, a := range scriptAnnotations {
			annotations[a.Key] = a.Value
		}
		// seed annotations override script annotations
		for _, b := range seedAnnotations {
			annotations[b.Key] = b.Value
		}

		initialArgs, err := json.Marshal(&annotations)
		if err != nil {
			log.Errorf("%v", err)
			continue
		}

		var eci runtime.ExecutionContextID
		// try to pick top level javascript execution context
		ecdLength := len(sess.ecd)
		for _, ecd := range sess.ecd {
			if ecd.Name == "" && strings.HasPrefix(sess.RequestedUrl.Uri, ecd.Origin) {
				eci = ecd.ID
				break
			}
		}
		if eci == 0 {
			log.Error("failed to get execution context")
			continue
		}

		// initalize function arguments
		arguments := json.RawMessage(initialArgs)
		// name of next script to run (self is current script)
		next := "self"

		for {
			var fn string
			if len(next) == 0 {
				break
			} else if next == "self" {
				fn = script.GetBrowserScript().GetScript()
				next = script.GetMeta().GetName()
			} else {
				var ok bool
				if fn, ok = undefinedScripts[next]; !ok {
					log.Errorf("No such next script: \"%s\"", next)
					break
				}
				annotations := make(map[string]interface{})
				// add seed annotations
				for _, b := range seedAnnotations {
					annotations[b.Key] = b.Value
				}
				// add annotations from next browserScript
				for _, annotation := range undefinedScriptAnnotations[next] {
					annotations[annotation.Key] = annotation.Value
				}
				var currentArguments map[string]interface{}
				err := json.Unmarshal(arguments, &currentArguments)
				if err != nil {
					// pass
				}
				// add currentArguments
				for key, value := range currentArguments {
					annotations[key] = value
				}
				arguments, err = json.Marshal(annotations)
				if err != nil {
					log.Errorf("failed")
					break
				}
			}
			if log.IsLevelEnabled(log.DebugLevel) {
				args := make(map[string]interface{})
				err = json.Unmarshal(arguments, &args)
				if err != nil {
					log.Errorf("failed to unmarshal arguments: %v", err)
				} else {
					log.Debugf("executing %s(%+v) in context %v", next, args, eci)
				}
			}

			_, isUpdateEcu := annotations["ecu"]
			// check if eg. a location change has caused creation of new execution contexts
			// we want the next script to run in new root context if created
			if isUpdateEcu && len(sess.ecd) > ecdLength {
				for _, ecd := range sess.ecd[ecdLength:] {
					// use the first new execution context created matching uri
					if ecd.Name == "" && strings.HasPrefix(sess.RequestedUrl.Uri, ecd.Origin) {
						eci = ecd.ID
						break
					}
				}
				ecdLength = len(sess.ecd)
			}

			result, err := sess.callScript(eci, fn, arguments)
			if err != nil {
				log.Errorf("failed to call script: %v", err)
				break
			}
			if result == nil {
				break
			}
			var rv ReturnValue
			err = json.Unmarshal(result, &rv)
			if err != nil {
				log.Errorf("failed to unmarshal return value: %v", err)
				break
			}

			if log.IsLevelEnabled(log.DebugLevel) {
				var d interface{}
				err = json.Unmarshal(rv.Data, &d)
				if err != nil {
					log.Errorf("failed to unmarshal data: %v", err)
				} else {
					log.Debugf("return {waitForCompletion: %t, next: %s, data: %+v}", rv.WaitForData, rv.Next, d)
				}
			}

			arguments = rv.Data
			next = rv.Next
			if rv.WaitForData {
				log.Trace("Wait for activity after script execution")
				waitStart := time.Now()
				_ = sess.netActivityTimer.WaitForCompletion()
				log.Tracef("Waited %v for network activity to settle", time.Since(waitStart))
				notifyCount := sess.netActivityTimer.Reset()
				log.Tracef("Got %d notifications while waiting for network activity to settle", notifyCount)
			}
		}
	}
}

// cleanWorkspace removes downloaded resources in browser container
func (sess *Session) cleanWorkspace() {
	if r, err := http.NewRequest("DELETE", sess.workspaceEndpoint+"/"+strconv.Itoa(sess.Id), nil); err != nil {
		log.Warnf("Error creating request for cleaning up workspace: %v", err)
	} else {
		if _, err = http.DefaultClient.Do(r); err != nil {
			log.Warnf("Error cleaning up workspace: %v", err)
		}
	}
}

func (sess *Session) getCookieParams(uri *frontierV1.QueuedUri) []*network.CookieParam {
	log.Debugf("Restoring %v browser cookies", len(uri.GetCookies()))
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
	var result []*frontierV1.Cookie

	if err := chromedp.Run(sess.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := network.GetAllCookies().Do(ctx)
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
		log.Errorf("Could not extract cookies: %v", err)
	}

	return result
}

func (sess *Session) saveScreenshot() {
	// Skip screenshot of pages loaded from cache
	if sess.Requests.RootRequest().FromCache {
		log.Debugf("Page with resource type %v is from cache, skipping screenshot", sess.Requests.RootRequest().ResourceType)
		return
	}

	// Check if page is renderable
	if sess.Requests.RootRequest().ResourceType != "Document" && sess.Requests.RootRequest().ResourceType != "Image" {
		log.Debugf("Page with resource type %v is not renderable, skipping screenshot", sess.Requests.RootRequest().ResourceType)
		return
	}

	var data []byte
	err := chromedp.Run(sess.ctx,
		chromedp.ActionFunc(func(ctx context.Context) (err error) {
			data, err = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatPng).Do(ctx)
			return
		}),
	)
	if err != nil {
		log.Errorf("Error capturing screenshot: %v", err)
		return
	}
	if err = sess.WriteScreenshot(sess.ctx, sess, data); err != nil {
		log.Errorf("Error writing screenshot: %v", err)
		return
	}
}

func (sess *Session) extractOutlinks() []*frontierV1.QueuedUri {
	cookies := sess.extractCookies()
	var extractedUrls []string
	for _, s := range sess.DbAdapter.GetScripts(sess.BrowserConfig, configV1.BrowserScript_EXTRACT_OUTLINKS) {
		log.Debugf("executing link extractor script")
		var res *runtime.RemoteObject
		var errDetail *runtime.ExceptionDetails
		err := chromedp.Run(sess.ctx,
			chromedp.ActionFunc(func(ctx context.Context) (err error) {
				res, errDetail, err = runtime.Evaluate(s.GetBrowserScript().GetScript()).
					WithReturnByValue(true).Do(ctx)
				return err
			}),
		)
		if err != nil {
			log.Warnf("evaluate script: %v %v", err, errDetail)
			continue
		}
		if res.Value != nil {
			var links []string
			err := json.Unmarshal(res.Value, &links)
			if err != nil {
				log.Warnf("unmarshal return value: %v", err)
				continue
			}

			log.Debugf("Found %d outlinks.", len(links))

			for _, link := range links {
				link = strings.TrimSpace(link)
				link = strings.Trim(link, "\"\\")
				if link != "" && link != sess.Requests.RootRequest().Url {
					extractedUrls = append(extractedUrls, link)
				}
			}
		}
	}

	outlinks := make([]*frontierV1.QueuedUri, len(extractedUrls))

	for _, uri := range extractedUrls {
		log.Tracef("Outlink: %v", uri)
		outlink := &frontierV1.QueuedUri{
			ExecutionId:         sess.RequestedUrl.ExecutionId,
			DiscoveredTimeStamp: ptypes.TimestampNow(),
			Uri:                 uri,
			DiscoveryPath:       sess.Requests.RootRequest().CrawlLog.DiscoveryPath + "L",
			Referrer:            sess.Requests.RootRequest().Url,
			Cookies:             cookies,
			JobExecutionId:      sess.RequestedUrl.JobExecutionId,
		}
		outlinks = append(outlinks, outlink)
	}

	return outlinks
}

func (sess *Session) AbortFetch() {
	log.Debugf("Aborting fetch")
	if err := chromedp.Run(sess.ctx,
		page.StopLoading(),
	); err != nil {
		log.Warnf("Error aborting fetch: %v", err)
	}
	sess.frameWg.Cancel()
	sess.loadCancel()
}
