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
	configV1 "github.com/nlnwa/veidemann-api-go/config/v1"
	"github.com/nlnwa/veidemann-browser-controller/pkg/database"
	"github.com/nlnwa/veidemann-browser-controller/pkg/script"
	log "github.com/sirupsen/logrus"
	"regexp"
	"time"
)

// executeScripts executes scripts of type scriptType.
func (sess *Session) executeScripts(ctx context.Context, scriptType configV1.BrowserScript_BrowserScriptType) error {
	// map of all scripts keyed on id
	scripts := make(map[string]*configV1.ConfigObject)

	// array of id's for scripts of given type
	var scriptIds []string

	// add all scripts of given type to scripts map and also add id to an array
	for _, configObject := range sess.DbAdapter.GetScripts(sess.BrowserConfig, scriptType) {
		if !match(configObject.GetBrowserScript().GetUrlRegexp(), sess.RequestedUrl.Uri) {
			continue
		}
		scripts[configObject.Id] = configObject
		scriptIds = append(scriptIds, configObject.Id)
	}
	if len(scriptIds) == 0 {
		return nil
	}
	seed := sess.DbAdapter.GetSeedByUri(sess.RequestedUrl)

	// add all potential next scripts to scripts map
	for _, configObject := range sess.DbAdapter.GetScripts(sess.BrowserConfig, configV1.BrowserScript_UNDEFINED) {
		if !match(configObject.GetBrowserScript().GetUrlRegexp(), sess.RequestedUrl.Uri) {
			continue
		}
		scripts[configObject.Id] = configObject
	}

	// wait is executed depending on value returned from script (WaitForData)
	wait := func() {
		waitStart := time.Now()
		_ = sess.netActivityTimer.WaitForCompletion()
		log.Debugf("Waited %v for network activity to settle", time.Since(waitStart))
		notifyCount := sess.netActivityTimer.Reset()
		log.Debugf("Got %d notifications while waiting for network activity to settle", notifyCount)
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

	execute := func(ctx context.Context, configObject *configV1.ConfigObject, arguments easyjson.RawMessage) (easyjson.RawMessage, error) {
		name := configObject.GetMeta().GetName()
		id := configObject.GetId()
		eci, err := resolveExecutionContextId()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve execution context id for script %s (%s): %w", name, id, err)
		}

		log.Debugf("Calling script %s (%s) in context %d with arguments %s", name, id, eci, arguments)

		res, err := callScript(ctx, eci, configObject.GetBrowserScript().GetScript(), arguments)

		log.Debugf("Script %s (%s) returned: %s", name, id, res)

		return res, err
	}

	for _, scriptId := range scriptIds {
		err := script.Run(ctx, scriptId, seed, scripts, execute, wait)
		if err != nil {
			return err
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
	normalizedUri := database.NormalizeUrl(uri)
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
