package session

import (
	"fmt"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/serviceworker"
	"github.com/chromedp/chromedp"
	"github.com/nlnwa/veidemann-browser-controller/pkg/database"
	"github.com/nlnwa/veidemann-browser-controller/pkg/requests"
	log "github.com/sirupsen/logrus"
)

func (sess *Session) initListeners() {
	chromedp.ListenTarget(sess.ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventRequestWillBeSent:
			if req := sess.Requests.GetByNetworkId(ev.RequestID.String()); req != nil {
				req.Initiator = ev.Initiator.Type.String()
			}
		case *network.EventLoadingFailed:
			log.Debugf("Fail: %v, %v, Reason; %v, Cancel: %v, %v, %v\n", ev.RequestID, ev.Type, ev.BlockedReason, ev.Canceled, ev.ErrorText, ev.Timestamp.Time())
		case *network.EventWebSocketCreated:
			log.Debugf("Sock create: %v %s\n", ev.RequestID, ev.URL)
		case *network.EventWebSocketClosed:
			log.Debugf("Sock close: %v\n", ev.RequestID)
		case *page.EventFrameStartedLoading:
			sess.Requests.NotifyLoadStart()
		case *page.EventFrameStoppedLoading:
			sess.Requests.NotifyLoadFinished()
		case *page.EventJavascriptDialogOpening:
			log.Debugf("javascript dialog opening", ev.Message)
			go func() {
				accept := false
				if ev.Type == "alert" {
					accept = true
				}
				if err := chromedp.Run(sess.ctx,
					page.HandleJavaScriptDialog(accept),
				); err != nil {
					panic(err)
				}
			}()
		case *serviceworker.EventWorkerRegistrationUpdated:
			go func() {
				if err := chromedp.Run(sess.ctx,
					serviceworker.StopAllWorkers(),
				); err != nil {
					panic(err)
				}
			}()
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
				if err := chromedp.Run(sess.ctx,
					continueRequest,
				); err != nil {
					log.Errorf("Failed sending continue: %v", err)
				}
			}()
		}
	})
}

func interfaceToString(i interface{}) string {
	if i == nil {
		return ""
	}
	return fmt.Sprintf("%v", i)
}
