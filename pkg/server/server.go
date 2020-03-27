package server

import (
	"context"
	"fmt"
	browsercontrollerV1 "github.com/nlnwa/veidemann-api-go/browsercontroller/v1"
	robotsevaluatorV1 "github.com/nlnwa/veidemann-api-go/robotsevaluator/v1"
	"github.com/nlnwa/veidemann-browser-controller/pkg/database"
	"github.com/nlnwa/veidemann-browser-controller/pkg/requests"
	"github.com/nlnwa/veidemann-browser-controller/pkg/session"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"io"
	"net"
)

// ApiServer is the gRPC api endpoint for the Browser Controller
type ApiServer struct {
	sessions   *session.SessionRegistry
	ln         net.Listener
	listenAddr net.Addr
	addr       string
	grpcServer *grpc.Server
}

// NewApiServer returns a new instance of ApiServer listening on the given port
func NewApiServer(listenInterface string, listenPort int, sessions *session.SessionRegistry) *ApiServer {
	a := &ApiServer{
		sessions: sessions,
		addr:     fmt.Sprintf("%s:%d", listenInterface, listenPort),
	}
	return a
}

func (a *ApiServer) Start() error {
	ln, err := net.Listen("tcp", a.addr)
	if err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}

	a.ln = ln
	a.listenAddr = ln.Addr()

	var opts []grpc.ServerOption
	a.grpcServer = grpc.NewServer(opts...)
	browsercontrollerV1.RegisterBrowserControllerServer(a.grpcServer, a)

	go func() {
		log.Infof("API server listening on address: %s", a.addr)
		a.grpcServer.Serve(ln)
	}()
	return nil
}

func (a *ApiServer) Close() {
	log.Infof("Shutting down API server")
	a.grpcServer.GracefulStop()
}

// Implements BrowserController
func (a *ApiServer) Do(server browsercontrollerV1.BrowserController_DoServer) error {
	var sess *session.Session
	var req *requests.Request

	for {
		request, err := server.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		switch v := request.Action.(type) {
		case *browsercontrollerV1.DoRequest_New:
			if v.New.ProxyId == 0 {
				sess, err = a.sessions.NewDirectSession(v.New.Uri, v.New.CrawlExecutionId, v.New.JobExecutionId)
				if err != nil {
					return fmt.Errorf("could not create session for 0-proxy %w", err)
				}
				_ = server.Send(&browsercontrollerV1.DoReply{
					Action: &browsercontrollerV1.DoReply_New{
						New: &browsercontrollerV1.NewReply{
							CrawlExecutionId: v.New.CrawlExecutionId,
							JobExecutionId:   v.New.JobExecutionId,
							CollectionRef:    v.New.CollectionRef,
						},
					},
				})
				continue
			} else {
				sess = a.sessions.Get(int(v.New.ProxyId))
			}

			if sess == nil {
				switch v.New.Method {
				case "CONNECT":
					// No session found, this is probably a CONNECT for robots.txt
					_ = server.Send(&browsercontrollerV1.DoReply{
						Action: &browsercontrollerV1.DoReply_New{
							New: &browsercontrollerV1.NewReply{
								CrawlExecutionId: v.New.CrawlExecutionId,
								JobExecutionId:   v.New.JobExecutionId,
								CollectionRef:    v.New.CollectionRef,
							},
						},
					})
				default:
					log.Infof("Cancelling Null session, proxy: %v, %v %v", v.New.ProxyId, v.New.Method, v.New.Uri)
					_ = server.Send(&browsercontrollerV1.DoReply{
						Action: &browsercontrollerV1.DoReply_Cancel{
							Cancel: "Cancelled by browser controller",
						},
					})
				}
				continue
			}

			sess.Notify()

			log.Tracef("Check robots for %v, jeid: %v, ceid: %v, policy: %v",
				v.New.Uri,
				sess.RequestedUrl.JobExecutionId,
				sess.RequestedUrl.ExecutionId,
				sess.PolitenessConfig.GetPolitenessConfig().RobotsPolicy)

			robotsRequest := &robotsevaluatorV1.IsAllowedRequest{
				JobExecutionId: sess.RequestedUrl.JobExecutionId,
				ExecutionId:    sess.RequestedUrl.ExecutionId,
				Uri:            v.New.Uri,
				UserAgent:      sess.UserAgent,
				Politeness:     sess.PolitenessConfig,
				CollectionRef:  sess.CrawlConfig.CollectionRef,
			}
			if !sess.RobotsIsAllowed(context.Background(), robotsRequest) {
				log.Debugf("URI %v was blocked by robots.txt", v.New.Uri)
				_ = server.Send(&browsercontrollerV1.DoReply{
					Action: &browsercontrollerV1.DoReply_Cancel{
						Cancel: "Blocked by robots.txt",
					},
				})
				continue
			}

			if v.New.RequestId == "" {
				switch v.New.Method {
				case "CONNECT":
					reply := &browsercontrollerV1.DoReply{
						Action: &browsercontrollerV1.DoReply_New{
							New: &browsercontrollerV1.NewReply{
								CrawlExecutionId: sess.RequestedUrl.ExecutionId,
								JobExecutionId:   sess.RequestedUrl.JobExecutionId,
								CollectionRef:    sess.CrawlConfig.CollectionRef,
							},
						},
					}
					_ = server.Send(reply)
					continue
				case "OPTIONS":
					Url := database.NormalizeUrl(v.New.Uri)
					req = sess.Requests.GetByUrl(Url, true)
					req.GotNew = true

				default:
					log.Warnf("New request from proxy without ID: %v %v", v.New.Method, v.New.Uri)
					_ = server.Send(&browsercontrollerV1.DoReply{
						Action: &browsercontrollerV1.DoReply_Cancel{
							Cancel: "Cancelled by browser controller",
						},
					})
					continue
				}
			} else {
				req = sess.Requests.GetByRequestId(v.New.RequestId)
				req.GotNew = true
			}

			reply := &browsercontrollerV1.NewReply{
				CrawlExecutionId: sess.RequestedUrl.ExecutionId,
				JobExecutionId:   sess.RequestedUrl.JobExecutionId,
				CollectionRef:    sess.CrawlConfig.CollectionRef,
			}
			replacementScript := sess.DbAdapter.GetReplacementScript(sess.BrowserConfig, v.New.Uri)
			if replacementScript != nil {
				reply.ReplacementScript = replacementScript
			}

			_ = server.Send(&browsercontrollerV1.DoReply{Action: &browsercontrollerV1.DoReply_New{New: reply}})
		case *browsercontrollerV1.DoRequest_Notify:
			if sess != nil {
				sess.Notify()
			} else {
				log.Warnf("Notify without session: %v %v\n", v.Notify.GetActivity(), req)
			}
		case *browsercontrollerV1.DoRequest_Completed:
			if sess == nil || (sess.Id != 0 && req == nil) {
				log.Infof("Missing session: %v %v %v", v.Completed.CrawlLog.WarcId, v.Completed.CrawlLog.Method, v.Completed.CrawlLog.RequestedUri)
			}
			if req == nil {
				if sess.Id == 0 && !v.Completed.Cached && v.Completed.CrawlLog != nil && v.Completed.CrawlLog.WarcId != "" {
					if err := sess.DbAdapter.WriteCrawlLog(v.Completed.CrawlLog); err != nil {
						log.Errorf("error writing crawlLog for direct session: %v", err)
					}
				} else {
					log.Errorf("Missing reqId for %v %v %v, Cached: %v", v.Completed.CrawlLog.Method, v.Completed.CrawlLog.StatusCode, v.Completed.CrawlLog.RequestedUri, v.Completed.Cached)
				}
			} else {
				req.CrawlLog = v.Completed.CrawlLog
				if v.Completed.Cached {
					if sess.Requests.InitialRequest().RequestId == req.RequestId {
						sess.AbortFetch()
					}
					req.FromCache = true
				}
				req.GotComplete = true
			}
			if sess != nil {
				sess.Notify()
			} else {
				log.Warnf("Notify without session: %v", req)
			}
		}
	}
}
