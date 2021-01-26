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
	"fmt"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/mailru/easyjson"
	configV1 "github.com/nlnwa/veidemann-api/go/config/v1"
	"github.com/nlnwa/veidemann-browser-controller/pkg/script"
	"github.com/nlnwa/veidemann-browser-controller/pkg/url"
	"github.com/opentracing/opentracing-go"
	tracelog "github.com/opentracing/opentracing-go/log"
	log "github.com/sirupsen/logrus"
	"regexp"
	"time"
)

type sessionScripts struct {
	scripts   map[configV1.BrowserScript_BrowserScriptType][]*configV1.ConfigObject
	blacklist []configV1.BrowserScript_BrowserScriptType
}

func newSessionScripts() *sessionScripts {
	return &sessionScripts{
		scripts: make(map[configV1.BrowserScript_BrowserScriptType][]*configV1.ConfigObject),
		blacklist: []configV1.BrowserScript_BrowserScriptType{
			configV1.BrowserScript_SCOPE_CHECK,
		},
	}
}

func (s *sessionScripts) IsBlacklisted(scriptType configV1.BrowserScript_BrowserScriptType) bool {
	for _, b := range s.blacklist {
		if scriptType == b {
			return true
		}
	}
	return false
}

func (s *sessionScripts) Get(scriptType configV1.BrowserScript_BrowserScriptType) []*configV1.ConfigObject {
	return s.scripts[scriptType]
}

func (sess *Session) loadScripts(ctx context.Context) (*sessionScripts, error) {
	bs := newSessionScripts()

	scripts, err := sess.DbAdapter.GetScripts(ctx, sess.browserConfig)
	if err != nil {
		return nil, err
	}
	for _, s := range scripts {
		scriptType := s.GetBrowserScript().GetBrowserScriptType()
		if bs.IsBlacklisted(scriptType) {
			continue
		}
		urlRegex := s.GetBrowserScript().GetUrlRegexp()
		if !match(urlRegex, sess.RequestedUrl.Uri) {
			continue
		}
		bs.scripts[scriptType] = append(bs.scripts[scriptType], s)
	}
	return bs, nil
}

func (sess *Session) GetReplacementScript(uri string) *configV1.BrowserScript {
	replacements := sess.scripts.Get(configV1.BrowserScript_REPLACEMENT)
	if len(replacements) == 0 {
		return nil
	}
	normalizedUri := url.Normalize(uri)
	longestMatch := 0
	var currentBestMatch *configV1.BrowserScript
	for _, bc := range replacements {
		for _, urlRegexp := range bc.GetBrowserScript().UrlRegexp {
			if re, err := regexp.Compile(urlRegexp); err == nil {
				re.Longest()
				l := len(re.FindString(normalizedUri))
				if l > 0 && l > longestMatch {
					longestMatch = l
					currentBestMatch = bc.GetBrowserScript()
				}
			} else {
				log.Warnf("Could not match url for replacement script %v", err)
			}
		}
	}
	return currentBestMatch
}

// executeScripts executes scripts of type scriptType.
func (sess *Session) executeScripts(ctx context.Context, scriptType configV1.BrowserScript_BrowserScriptType) error {
	// wait is executed depending on value returned from script (WaitForData)
	wait := func() {
		waitStart := time.Now()
		_ = sess.netActivityTimer.WaitForCompletion()
		log.Tracef("Waited %v for network activity to settle", time.Since(waitStart))
		notifyCount := sess.netActivityTimer.Reset()
		log.Tracef("Got %d notifications while waiting for network activity to settle", notifyCount)
	}

	var resolveExecutionContextId func() (runtime.ExecutionContextID, error)
	switch scriptType {
	case configV1.BrowserScript_ON_LOAD:
		// reuse the same execution context for all scripts
		executionContextID, err := getExecutionContextID(ctx)
		if err != nil {
			return fmt.Errorf("failed to get isolated execution context id: %w", err)
		}
		resolveExecutionContextId = func() (runtime.ExecutionContextID, error) {
			return executionContextID, nil
		}
	case configV1.BrowserScript_ON_NEW_DOCUMENT:
		// create new execution context before execution of scripts
		resolveExecutionContextId = func() (runtime.ExecutionContextID, error) {
			return getExecutionContextID(ctx)
		}
	default:
		return fmt.Errorf("script execution for type %v is not implemented", scriptType)
	}

	execute := func(configObject *configV1.ConfigObject, arguments easyjson.RawMessage) (easyjson.RawMessage, error) {
		span, ctx := opentracing.StartSpanFromContext(ctx, "execute script")
		defer span.Finish()
		name := configObject.GetMeta().GetName()
		id := configObject.GetId()
		eci, err := resolveExecutionContextId()
		if err != nil {
			span.SetTag("error", true).LogFields(tracelog.Event("error"), tracelog.Error(err))
			return nil, fmt.Errorf("failed to resolve execution context id for script %s (%s): %w", name, id, err)
		}
		span.SetTag("script.name", name).SetTag("script.id", id).SetTag("script.eci", eci)
		log.Debugf("Calling script %s (%s) in context %d with arguments %s", name, id, eci, arguments)

		res, err := callScript(ctx, eci, configObject.GetBrowserScript().GetScript(), arguments)
		if err != nil {
			span.SetTag("error", true).LogFields(tracelog.Event("error"), tracelog.Error(err))
		}
		log.Debugf("Script %s (%s) returned: %s", name, id, res)

		return res, err
	}
	scripts := make(map[string]*configV1.ConfigObject)
	for _, s := range sess.scripts.Get(configV1.BrowserScript_UNDEFINED) {
		scripts[s.Id] = s
	}
	for _, s := range sess.scripts.Get(scriptType) {
		// add initial script to map
		scripts[s.Id] = s
		err := script.Run(s.Id, scripts, sess.RequestedUrl.Annotation, execute, wait)
		if err != nil {
			return fmt.Errorf("failed to run script %s (%s): %w", s.Meta.Name, s.Id, err)
		}
	}
	return nil
}

// callScript runs a script function in the given execution context using the
// provided arguments  by the chrome debug protocol.
//
// The result value from the debug protocol action is unmarshalled into a
// ReturnValue struct.
//
// Returns an error: if the debug protocol action fails, if script execution
// caused an exception, or if unmarshalling of result value fails.
func callScript(ctx context.Context, eci runtime.ExecutionContextID, functionDeclaration string, arguments easyjson.RawMessage) (easyjson.RawMessage, error) {
	var res *runtime.RemoteObject
	var exceptionDetails *runtime.ExceptionDetails
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) (err error) {
			res, exceptionDetails, err = runtime.
				CallFunctionOn(functionDeclaration).
				WithArguments([]*runtime.CallArgument{{Value: arguments}}).
				WithExecutionContextID(eci).
				WithReturnByValue(true).
				Do(ctx)
			return err
		}),
	)
	if err != nil {
		return nil, err
	}
	if exceptionDetails != nil {
		return nil, exceptionDetails
	}

	return res.Value, nil
}

// getExecutionContextID creates an isolated world from the root frame and returns
// an execution context id.
func getExecutionContextID(ctx context.Context) (runtime.ExecutionContextID, error) {
	frameTree, err := getFrameTree(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get frameTree: %w", err)
	}

	var eci runtime.ExecutionContextID
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		eci, err = page.CreateIsolatedWorld(frameTree.Frame.ID).Do(ctx)
		return err
	}))
	if err != nil {
		return 0, fmt.Errorf("failed to create isolated world: %w", err)
	}
	return eci, nil
}

// getFrameTree returns the frame tree of the current page or an error if it fails.
func getFrameTree(ctx context.Context) (*page.FrameTree, error) {
	var frameTree *page.FrameTree
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		frameTree, err = page.GetFrameTree().Do(ctx)
		return err
	}))
	if err != nil {
		return nil, err
	}
	return frameTree, nil
}

// match takes an array of regular expressions and an URI. It returns true if
// a normalized version of the URI matches any of the regular expressions or
// if the array of regular expressions is empty, and false otherwise.
func match(regExps []string, uri string) bool {
	if len(regExps) == 0 {
		return true
	}
	normalizedUri := url.Normalize(uri)
	match := false
	for _, urlRegexp := range regExps {
		re, err := regexp.Compile(urlRegexp)
		if err != nil {
			log.Warnf("Failed to compile regexp: %s", urlRegexp)
			continue
		}
		match = re.MatchString(normalizedUri)
		if match {
			break
		}
	}
	return match
}
