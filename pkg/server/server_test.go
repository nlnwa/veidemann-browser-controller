package server

import (
	"bytes"
	"context"
	"fmt"
	configV1 "github.com/nlnwa/veidemann-api-go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api-go/frontier/v1"
	"github.com/nlnwa/veidemann-api-go/robotsevaluator/v1"
	"github.com/nlnwa/veidemann-browser-controller/pkg/database"
	"github.com/nlnwa/veidemann-browser-controller/pkg/session"
	"github.com/nlnwa/veidemann-recorderproxy/recorderproxy"
	"github.com/nlnwa/veidemann-recorderproxy/serviceconnections"
	"github.com/nlnwa/veidemann-recorderproxy/testutil"
	"github.com/ory/dockertest"
	"github.com/sirupsen/logrus"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

var sessions *session.SessionRegistry

var localhost = GetOutboundIP().String()

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.WarnLevel)

	// uses a sensible default on windows (tcp/http) and linux/osx (socket)
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	browserContainer, browserPort := setupBrowser(pool)

	dbAdapter := setupDbMock()
	sessions = session.NewSessionRegistry(
		context.Background(),
		2,
		session.WithBrowserPort(browserPort),
		session.WithProxyHost(localhost),
		session.WithProxyPort(6666),
		session.WithDbAdapter(dbAdapter),
		session.WithIsAllowedByRobotsTxtFunc(func(ctx context.Context, request *robotsevaluator.IsAllowedRequest) bool {
			fmt.Printf("ROBOTS %v, %v, %v, %v, %v\n", request.ExecutionId, request.JobExecutionId, request.CollectionRef, request.Politeness != nil, request.Uri)
			return true
		}),
		session.WithWriteScreenshotFunc(func(ctx context.Context, sess *session.Session, data []byte) error {
			b := bytes.NewBuffer(data)
			f, e := os.Create("screenshot.png")
			if e != nil {
				log.Fatal("Error opening file %w", e)
			}
			defer f.Close()
			io.Copy(f, b)
			return nil
		}),
	)
	apiServer := NewApiServer("", 7777, sessions)
	apiServer.Start()

	// Setup recorder proxy
	opt := testutil.WithExternalBrowserController(serviceconnections.NewConnectionOptions("BrowserController",
		serviceconnections.WithHost("localhost"),
		serviceconnections.WithPort("7777"),
		serviceconnections.WithConnectTimeout(10*time.Second),
	))
	grpcServices := testutil.NewGrpcServiceMock(opt)
	recorderProxy0 := localRecorderProxy(0, grpcServices.ClientConn, "")
	recorderProxy1 := localRecorderProxy(1, grpcServices.ClientConn, "")
	recorderProxy2 := localRecorderProxy(2, grpcServices.ClientConn, "")

	//logrus.SetLevel(logrus.InfoLevel)
	// Run the tests
	code := m.Run()

	// Clean up
	// You can't defer this because os.Exit doesn't care for defer
	if err := pool.Purge(browserContainer); err != nil {
		log.Fatalf("Could not purge browserContainer: %s", err)
	}

	apiServer.Close()
	grpcServices.Close()
	recorderProxy0.Close()
	recorderProxy1.Close()
	recorderProxy2.Close()

	os.Remove("screenshot.png")
	os.Remove("crawl.log")
	os.Remove("page.log")
	os.Exit(code)
}

func TestSession_Fetch(t *testing.T) {
	conf := &configV1.ConfigObject{
		Id:         "conf1",
		ApiVersion: "v1",
		Kind:       configV1.Kind_crawlConfig,
		Meta:       nil,
		Spec: &configV1.ConfigObject_CrawlConfig{CrawlConfig: &configV1.CrawlConfig{
			BrowserConfigRef: &configV1.ConfigRef{Id: "browserConfig1"},
			PolitenessRef:    &configV1.ConfigRef{Id: "politenessConfig1"},
			CollectionRef:    &configV1.ConfigRef{Id: "collectionConfig1"},
			Extra:            &configV1.ExtraConfig{CreateScreenshot: true},
		}},
	}

	tests := []struct {
		name string
		url  *frontierV1.QueuedUri
	}{
		{"1", &frontierV1.QueuedUri{Uri: "http://elg.no", DiscoveryPath: "L"}},
		//{"2", &frontierV1.QueuedUri{Uri: "http://vg.no", DiscoveryPath: "L"}},
		//{"3", &frontierV1.QueuedUri{Uri: "http://nb.no", DiscoveryPath: "L"}},
		//{"4", &frontierV1.QueuedUri{Uri: "http://fhi.no", DiscoveryPath: "L"}},
		//{"5", &frontierV1.QueuedUri{Uri: "http://db.no", DiscoveryPath: "L", JobExecutionId:"jid", ExecutionId:"eid"}},
		//{"5", &frontierV1.QueuedUri{Uri: "https://goo.gl/maps/EmpIH", DiscoveryPath: "L", JobExecutionId:"jid", ExecutionId:"eid"}},
		//{"5", &frontierV1.QueuedUri{Uri: "https://ranano.no/", DiscoveryPath: "L"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := sessions.GetNextAvailable()
			if err != nil {
				t.Error(err)
			}
			_, err = s.Fetch(tt.url, conf)
			if err != nil {
				t.Error(err)
			}
			//result, err := s.Fetch(tt.url, conf)
			//fmt.Printf("Err: %v\n", err)
			//b, _ := json.Marshal(result)
			//fmt.Printf("Result: %s\n", b)
			sessions.Release(s)
		})
	}
}

func setupDbMock() *database.DbAdapter {
	dbConn := database.NewMock()
	dbConn.GetMock().On(r.Table("config").Get("browserConfig1")).Return(
		map[string]interface{}{
			"id":   "browserConfig1",
			"kind": "browserConfig",
			"meta": map[string]interface{}{
				"name":    "browser config 1",
				"label":   []map[string]interface{}{{"key": "foo", "value": "bar"}},
				"created": "2020-04-06T18:17:50.343827619Z",
			},
			"browserConfig": map[string]interface{}{
				"windowWidth":         1400,
				"windowHeight":        1280,
				"maxInactivityTimeMs": 8000,
				"pageLoadTimeoutMs":   30000,
				"scriptRef":           []map[string]interface{}{{"kind": "browserScript", "id": "script1"}},
			},
		},
		nil,
	)
	dbConn.GetMock().On(r.Table("config").Get("script1")).Return(
		map[string]interface{}{
			"id":   "script1",
			"kind": "browserScript",
			"meta": map[string]interface{}{
				"name":        "script1",
				"description": "script1",
				"label":       []map[string]interface{}{{"key": "type", "value": "extract_outlinks"}},
			},
			"browserScript": map[string]interface{}{
				"script": `var __brzl_framesDone = new Set();
var __brzl_compileOutlinks = function (frame) {
    __brzl_framesDone.add(frame);
    if (frame && frame.document) {
        var outlinks = Array.prototype.slice.call(frame.document.querySelectorAll('a[href]'));
        for (var i = 0; i < frame.frames.length; i++) {
            if (frame.frames[i] && !__brzl_framesDone.has(frame.frames[i])) {
                try {
                    outlinks = outlinks.concat(__brzl_compileOutlinks(frame.frames[i]));
                } catch {
                }
            }
        }
    }
    return outlinks;
}
__brzl_compileOutlinks(window).join('\\n');`,
			},
		},
		nil,
	)
	dbConn.GetMock().On(r.Table("config").Get("politenessConfig1")).Return(
		map[string]interface{}{
			"id":   "politenessConfig1",
			"kind": "politenessConfig",
			"meta": map[string]interface{}{
				"name":    "politeness config 1",
				"label":   []map[string]interface{}{{"key": "foo", "value": "bar"}},
				"created": "2020-04-06T18:17:50.343827619Z",
			},
			"politenessConfig": map[string]interface{}{},
		}, nil)
	dbConn.GetMock().On(r.Table("page_log").Insert(r.MockAnything())).Return(map[string]interface{}{}, nil)
	dbConn.GetMock().On(r.Table("crawl_log").Insert(r.MockAnything())).Return(map[string]interface{}{}, nil)

	return database.NewDbAdapter(dbConn, time.Minute)
}

// localRecorderProxy creates a new recorderproxy which uses internal transport
func localRecorderProxy(id int, conn *serviceconnections.Connections, nextProxyAddr string) (proxy *recorderproxy.RecorderProxy) {
	proxy = recorderproxy.NewRecorderProxy(id, "0.0.0.0", 6666, conn, 1*time.Minute, nextProxyAddr)
	proxy.Start()
	return
}

// Get preferred outbound ip of this machine
func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP
}

func setupBrowser(pool *dockertest.Pool) (container *dockertest.Resource, port int) {
	var err error
	// pulls an image, creates a container based on it and runs it
	container, err = pool.Run("browserless/chrome", "1.29.0-puppeteer-2.1.1", []string{})
	if err != nil {
		log.Fatalf("Could not start browserContainer: %s", err)
	}

	port, err = strconv.Atoi(container.GetPort("3000/tcp"))
	if err != nil {
		log.Fatalf("Could not get port for browserContainer: %s", err)
	}

	// exponential backoff-retry, because the application in the container might not be ready to accept connections yet
	if err := pool.Retry(func() error {
		_, err := http.Head("http://localhost:" + container.GetPort("3000/tcp"))
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}
	return
}

func setupRobotsEvaluator(pool *dockertest.Pool) (container *dockertest.Resource, port int) {
	var err error
	// pulls an image, creates a container based on it and runs it
	container, err = pool.Run("norsknettarkiv/veidemann-robotsevaluator-service", "0.3.8", []string{})
	if err != nil {
		log.Fatalf("Could not start robotsContainer: %s", err)
	}

	port, err = strconv.Atoi(container.GetPort("7053/tcp"))
	if err != nil {
		log.Fatalf("Could not get port for robotsContainer: %s", err)
	}

	// exponential backoff-retry, because the application in the container might not be ready to accept connections yet
	if err := pool.Retry(func() error {
		_, err := http.Head("http://localhost:" + container.GetPort("7053/tcp"))
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}
	return
}
