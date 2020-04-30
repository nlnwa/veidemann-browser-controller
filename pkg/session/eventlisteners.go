package session

import (
	"context"
	"fmt"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/security"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/nlnwa/veidemann-browser-controller/pkg/database"
	"github.com/nlnwa/veidemann-browser-controller/pkg/requests"
	log "github.com/sirupsen/logrus"
)

func (sess *Session) initListeners() {
	chromedp.ListenTarget(sess.ctx, sess.listenFunc(sess.ctx))
}

func (sess *Session) listenFunc(ctx context.Context) func(ev interface{}) {
	return func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventRequestWillBeSent:
			log.Tracef("Request will be sent: %v, %v, %v, %v, %v, %v", ev.RequestID, ev.Type, ev.FrameID, ev.Initiator.Type, ev.LoaderID, ev.DocumentURL)
			if req := sess.Requests.GetByNetworkId(ev.RequestID.String()); req != nil {
				req.Initiator = ev.Initiator.Type.String()
			}
		case *network.EventLoadingFailed:
			log.Debugf("Loading failed: %v, %v, Reason; %v, Cancel: %v, %v, %v", ev.RequestID, ev.Type, ev.BlockedReason, ev.Canceled, ev.ErrorText, ev.Timestamp.Time())
		case *page.EventFrameStartedLoading:
			log.Tracef("Frame started loading: %v", ev.FrameID)
			sess.Requests.NotifyLoadStart()
		case *page.EventFrameStoppedLoading:
			log.Tracef("Frame stopped loading: %v", ev.FrameID)
			sess.Requests.NotifyLoadFinished()
		case *page.EventFileChooserOpened:
			log.Warnf("File chooser opened: %v %v %v", ev.BackendNodeID, ev.FrameID, ev.Mode)
		case *page.EventDownloadWillBegin:
			log.Tracef("Download will begin: %v %v", ev.FrameID, ev.URL)
		case *page.EventJavascriptDialogOpening:
			log.Debugf("javascript dialog opening %v", ev.Message)
			go func() {
				accept := false
				if ev.Type == "alert" {
					accept = true
				}
				if err := chromedp.Run(ctx,
					page.HandleJavaScriptDialog(accept),
				); err != nil {
					log.Errorf("Could not handle JavaScript dialog: %v", err)
				}
			}()
		case *target.EventTargetCreated:
			log.Tracef("Target created: %v :: %v :: %v :: %v :: %v :: %v :: %v\n", ev.TargetInfo.TargetID, ev.TargetInfo.OpenerID, ev.TargetInfo.BrowserContextID, ev.TargetInfo.Type, ev.TargetInfo.Title, ev.TargetInfo.URL, ev.TargetInfo.Attached)
			newCtx, _ := chromedp.NewContext(ctx, chromedp.WithTargetID(ev.TargetInfo.TargetID))
			go func() {
				select {
				case <-ctx.Done():
					_ = chromedp.Cancel(newCtx)
				}
			}()
			if err := chromedp.Run(newCtx); err != nil {
				log.Warnf("Failed connecting to new target: %v", err)
			}

			var actions []chromedp.Action

			switch ev.TargetInfo.Type {
			case "service_worker":
				actions = []chromedp.Action{
					fetch.Enable(),
					runtime.Enable(),
					target.SetAutoAttach(true, false).WithFlatten(true),
					runtime.RunIfWaitingForDebugger(),
					network.SetCacheDisabled(true),
					network.SetCookies(sess.getCookieParams(sess.RequestedUrl)),
				}
			case "worker":
				actions = []chromedp.Action{
					runtime.Enable(),
					target.SetAutoAttach(true, false).WithFlatten(true),
					runtime.RunIfWaitingForDebugger(),
					network.SetCacheDisabled(true),
				}
			default:
				actions = []chromedp.Action{
					fetch.Enable(),
					runtime.Enable(),
					target.SetAutoAttach(true, false).WithFlatten(true),
					runtime.RunIfWaitingForDebugger(),
					network.Enable(),
					page.Enable(),
					network.SetCacheDisabled(true),
					security.SetIgnoreCertificateErrors(true),
					network.SetCookies(sess.getCookieParams(sess.RequestedUrl)),
				}
			}

			go func() {
				if err := chromedp.Run(newCtx, actions...); err != nil {
					log.Errorf("Failed initializing new target: %v", err)
				}

				chromedp.ListenTarget(newCtx, sess.listenFunc(newCtx))
			}()
			sess.Notify(ev.TargetInfo.TargetID.String())
		case *fetch.EventRequestPaused:
			go func() {
				continueRequest := fetch.ContinueRequest(ev.RequestID)
				if ev.ResponseStatusCode == 0 && ev.ResponseErrorReason == "" {
					continueRequest = continueRequest.WithURL(ev.Request.URL).WithMethod(ev.Request.Method)
					req := &requests.Request{
						Method:       ev.Request.Method,
						Url:          database.NormalizeUrl(ev.Request.URL + ev.Request.URLFragment),
						RequestId:    ev.RequestID.String(),
						NetworkId:    ev.NetworkID.String(),
						Referrer:     interfaceToString(ev.Request.Headers["Referer"]),
						ResourceType: ev.ResourceType.String(),
					}

					sess.Requests.AddRequest(req)

					if ev.Request.Headers["veidemann_reqid"] != nil {
						delete(ev.Request.Headers, "veidemann_reqid")
					}
					h := make([]*fetch.HeaderEntry, len(ev.Request.Headers)+1)
					i := 0
					for k, v := range ev.Request.Headers {
						h[i] = &fetch.HeaderEntry{Name: k, Value: interfaceToString(v)}
						i++
					}
					h[i] = &fetch.HeaderEntry{Name: "veidemann_reqid", Value: ev.RequestID.String()}
					continueRequest = continueRequest.WithHeaders(h)
				} else {
					log.Infof("RESPONSE REQUEST %v %v %v\n", ev.ResponseStatusCode, ev.ResponseErrorReason, ev.Request.URL)
				}
				if err := chromedp.Run(ctx,
					continueRequest,
				); err != nil {
					log.Debugf("Failed sending continue: %v", err)
				} else {
					sess.Notify(ev.RequestID.String())
				}
			}()
		}
	}
}

func interfaceToString(i interface{}) string {
	if i == nil {
		return ""
	}
	return fmt.Sprintf("%v", i)
}
