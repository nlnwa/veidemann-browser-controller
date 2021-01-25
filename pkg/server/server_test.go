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

package server

import (
	"bytes"
	"context"
	"fmt"
	configV1 "github.com/nlnwa/veidemann-api/go/config/v1"
	frontierV1 "github.com/nlnwa/veidemann-api/go/frontier/v1"
	robotsevaluatorV1 "github.com/nlnwa/veidemann-api/go/robotsevaluator/v1"
	"github.com/nlnwa/veidemann-browser-controller/pkg/database"
	"github.com/nlnwa/veidemann-browser-controller/pkg/screenshotwriter"
	"github.com/nlnwa/veidemann-browser-controller/pkg/session"
	"github.com/nlnwa/veidemann-browser-controller/pkg/testutil"
	"github.com/nlnwa/veidemann-recorderproxy/recorderproxy"
	"github.com/nlnwa/veidemann-recorderproxy/serviceconnections"
	proxyTestUtil "github.com/nlnwa/veidemann-recorderproxy/testutil"
	"github.com/ory/dockertest"
	log "github.com/sirupsen/logrus"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

var sessions *session.Registry

var localhost = GetOutboundIP().String()

func TestMain(m *testing.M) {
	log.SetLevel(log.DebugLevel)

	// uses a sensible default on windows (tcp/http) and linux/osx (socket)
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}
	browserContainer, browserPort := setupBrowser(pool)

	dbMock := setupDbMock()
	dbAdapter := database.NewDbAdapter(dbMock, time.Minute)
	screenShotWriter := &testutil.ScreenshotWriterMock{
		WriteFunc: func(data []byte, metadata screenshotwriter.Metadata) error {
			b := bytes.NewBuffer(data)
			f, err := os.Create("screenshot.png")
			if err != nil {
				return fmt.Errorf("error opening file: %w", err)
			}
			defer func() {
				_ = f.Close()
			}()
			_, err = io.Copy(f, b)
			if err != nil {
				return fmt.Errorf("failed to copy screenshot data to file: %w", err)
			}
			return nil
		},
		CloseFunc: func() {
			_ = os.Remove("screenshot.png")
		},
	}
	sessions = session.NewRegistry(
		2,
		session.WithBrowserPort(browserPort),
		session.WithProxyHost(localhost),
		session.WithProxyPort(6666),
		session.WithDbAdapter(dbAdapter),
		session.WithScreenshotWriter(screenShotWriter),
	)

	robotsEvaluator := &testutil.RobotsEvaluatorMock{IsAllowedFunc: func(_ *robotsevaluatorV1.IsAllowedRequest) bool {
		return true
	}}
	apiServer := NewApiServer("", 7777, sessions, robotsEvaluator)
	go func() {
		_ = apiServer.Start()
	}()

	// Setup recorder proxy
	opt := proxyTestUtil.WithExternalBrowserController(serviceconnections.NewConnectionOptions("BrowserController",
		serviceconnections.WithHost("localhost"),
		serviceconnections.WithPort("7777"),
		serviceconnections.WithConnectTimeout(10*time.Second),
	))
	grpcServices := proxyTestUtil.NewGrpcServiceMock(opt)
	recorderProxy0 := localRecorderProxy(0, grpcServices.ClientConn, "")
	recorderProxy1 := localRecorderProxy(1, grpcServices.ClientConn, "")
	recorderProxy2 := localRecorderProxy(2, grpcServices.ClientConn, "")

	// Run the tests
	code := m.Run()

	// Clean up
	sessions.CloseWait(1 * time.Minute)
	if err := pool.Purge(browserContainer); err != nil {
		log.Warnf("Could not purge browserContainer: %s", err)
	}
	apiServer.Close()
	grpcServices.Close()
	recorderProxy0.Close()
	recorderProxy1.Close()
	recorderProxy2.Close()
	screenShotWriter.Close()
	_ = dbMock.Close()

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
		{"elg", &frontierV1.QueuedUri{Uri: "http://elg.no", DiscoveryPath: "L", JobExecutionId: "jid", ExecutionId: "eid"}},
		{"vg", &frontierV1.QueuedUri{Uri: "http://vg.no", DiscoveryPath: "L", JobExecutionId: "jid", ExecutionId: "eid"}},
		{"nb", &frontierV1.QueuedUri{Uri: "http://nb.no", DiscoveryPath: "L", JobExecutionId: "jid", ExecutionId: "eid"}},
		{"fhi", &frontierV1.QueuedUri{Uri: "http://fhi.no", DiscoveryPath: "L", JobExecutionId: "jid", ExecutionId: "eid"}},
		{"db", &frontierV1.QueuedUri{Uri: "http://db.no", DiscoveryPath: "L", JobExecutionId: "jid", ExecutionId: "eid"}},
		{"maps", &frontierV1.QueuedUri{Uri: "https://goo.gl/maps/EmpIH", DiscoveryPath: "L", JobExecutionId: "jid", ExecutionId: "eid"}},
		{"ranano", &frontierV1.QueuedUri{Uri: "https://ranano.no/", DiscoveryPath: "L", JobExecutionId: "jid", ExecutionId: "eid"}},
		{"cynergi", &frontierV1.QueuedUri{Uri: "https://cynergi.no/", DiscoveryPath: "L", JobExecutionId: "jid", ExecutionId: "eid"}},
		{"pdf1", &frontierV1.QueuedUri{Uri: "https://spaniaposten.no/spaniaposten.no/pdf/2004/54-SpaniaPosten.pdf", DiscoveryPath: "L", JobExecutionId: "jid", ExecutionId: "eid"}},
		{"pdf2", &frontierV1.QueuedUri{Uri: "http://publikasjoner.nve.no/rapport/2015/rapport2015_89.pdf", DiscoveryPath: "L", JobExecutionId: "jid", ExecutionId: "eid"}},
	}
	for _, tt := range tests {
		ctx := context.Background()
		t.Run(tt.name, func(t *testing.T) {
			s, err := sessions.GetNextAvailable(ctx)
			if err != nil {
				t.Error(err)
			}
			result, err := s.Fetch(tt.url, conf)
			if err != nil {
				t.Error(err)
			} else {
				t.Logf("Resource count: %v, Time: %v\n", result.UriCount, result.PageFetchTimeMs)
			}
			sessions.Release(s)
			//time.Sleep(time.Second*4)
		})
	}
}

func setupDbMock() *database.MockConnection {
	dbConn := database.NewMockConnection().(*database.MockConnection)
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
				"maxInactivityTimeMs": 5000,
				"pageLoadTimeoutMs":   60000,
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
				"browserScriptType": "EXTRACT_OUTLINKS",
				"script": `    (function extractOutlinks(frame) {
      const framesDone = new Set();
      function isValid(link) {
      return (link != null
            && link.attributes.href.value != ""
            && link.attributes.href.value != "#"
            && link.protocol != "tel:"
            && link.protocol != "mailto:"
           );
      }
      function compileOutlinks(frame) {
        framesDone.add(frame);
        if (frame && frame.document) {
          let outlinks = Array.from(frame.document.links);
          for (var i = 0; i < frame.frames.length; i++) {
            if (frame.frames[i] && !framesDone.has(frame.frames[i])) {
              try {
                outlinks = outlinks.concat(compileOutlinks(frame.frames[i]));
              } catch {}
            }
          }
          return outlinks;
        }
        return [];
      }
      return Array.from(new Set(compileOutlinks(frame).filter(isValid).map(_ => _.href)));
    })(window);`,
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

	return dbConn
}

// localRecorderProxy creates a new recorderproxy which uses internal transport
func localRecorderProxy(id int, conn *serviceconnections.Connections, nextProxyAddr string) (proxy *recorderproxy.RecorderProxy) {
	proxy = recorderproxy.NewRecorderProxy(id, "0.0.0.0", 6666, conn, 5*time.Second, nextProxyAddr)
	proxy.Start()
	return
}

// Get preferred outbound ip of this machine
func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = conn.Close()
	}()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP
}

func setupBrowser(pool *dockertest.Pool) (container *dockertest.Resource, port int) {
	var err error
	// pulls an image, creates a container based on it and runs it
	//container, err = pool.Run("browserless/chrome", "1.32.0-puppeteer-2.1.1", []string{"WORKSPACE_DIR", "/dev/null"})//, "DISABLE_AUTO_SET_DOWNLOAD_BEHAVIOR", "true"})
	container, err = pool.Run("browserless/chrome", "1.33.0-puppeteer-3.0.0", []string{})
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
