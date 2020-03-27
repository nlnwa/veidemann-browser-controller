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

package main

import (
	"github.com/nlnwa/veidemann-browser-controller/pkg/controller"
	"github.com/nlnwa/veidemann-browser-controller/pkg/database"
	"github.com/nlnwa/veidemann-browser-controller/pkg/logger"
	"github.com/nlnwa/veidemann-browser-controller/pkg/metrics"
	"github.com/nlnwa/veidemann-browser-controller/pkg/serviceconnections"
	"github.com/nlnwa/veidemann-browser-controller/pkg/session"
	"github.com/nlnwa/veidemann-browser-controller/pkg/tracing"
	"github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	pflag.BoolP("help", "h", false, "Usage instructions")
	pflag.String("interface", "", "interface the browser controller api listens to. No value means all interfaces.")
	pflag.Int("port", 8080, "port the browser controller api listens to.")
	pflag.Int("proxy-count", 10, "max number of simultanious sessions. Must match RecorderProxy's proxy-count setting.")
	pflag.String("browser-host", "localhost", "Content writer host")
	pflag.Int("browser-port", 3000, "Content writer port")
	pflag.String("proxy-host", "localhost", "Content writer host")
	pflag.String("proxy-port", "9900", "Content writer port")
	pflag.String("content-writer-host", "veidemann-contentwriter", "Content writer host")
	pflag.String("content-writer-port", "8082", "Content writer port")
	pflag.String("frontier-host", "veidemann-frontier", "DNS resolver host")
	pflag.String("frontier-port", "7700", "DNS resolver port")
	pflag.String("robots-evaluator-host", "veidemann-robotsevaluator-service", "Browser controller host")
	pflag.String("robots-evaluator-port", "7053", "Browser controller port")
	pflag.Duration("connect-timeout", 1*time.Minute, "Timeout used for connecting to GRPC services")
	pflag.String("db-host", "rethinkdb-proxy", "Path to CA certificate used for signing client connections")
	pflag.String("db-port", "28015", "Path to CA certificate used for signing client connections")
	pflag.String("db-name", "veidemann", "Path to CA certificate used for signing client connections")
	pflag.String("db-user", "", "Path to CA certificate used for signing client connections")
	pflag.String("db-password", "", "Path to CA certificate used for signing client connections")

	pflag.String("metrics-interface", "", "Path to CA certificate used for signing client connections")
	pflag.Int("metrics-port", 9153, "Path to CA certificate used for signing client connections")
	pflag.String("metrics-path", "/metrics", "Path to CA certificate used for signing client connections")

	pflag.String("log-level", "trace", "log level, available levels are panic, fatal, error, warn, info, debug and trace")
	pflag.String("log-formatter", "logfmt", "log formatter, available values are logfmt and json")
	pflag.Bool("log-method", false, "log method name")

	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	viper.SetDefault("ContentDir", "content")
	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.AutomaticEnv()
	err := viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		log.Fatalf("Could not parse flags: %s", err)
	}

	if viper.GetBool("help") {
		pflag.Usage()
		return
	}

	if err := logger.InitLog(
		viper.GetString("log-level"),
		viper.GetString("log-formatter"),
		viper.GetBool("log-method"),
	); err != nil {
		log.Fatalf("Could not initialize logs: %v", err)
	}

	ms := metrics.NewMetricsServer(viper.GetString("metrics-interface"), viper.GetInt("metrics-port"), viper.GetString("metrics-path"))
	if err := ms.Start(); err != nil {
		log.Fatalf("Could not start metrics server: %v", err)
	}
	defer ms.Close()

	tracer, closer := tracing.Init("Recorder Proxy")
	if tracer != nil {
		opentracing.SetGlobalTracer(tracer)
		defer closer.Close()
	}

	connectTimeout := viper.GetDuration("connect-timeout")

	contentWriterConn := serviceconnections.NewContentWriterConn(
		serviceconnections.WithConnectTimeout(connectTimeout),
		serviceconnections.WithHost(viper.GetString("content-writer-host")),
		serviceconnections.WithPort(viper.GetString("content-writer-port")),
	)
	if err := contentWriterConn.Connect(); err != nil {
		log.Fatalf("Could not connect to content writer: %v", err)
	}
	frontierConn := serviceconnections.NewFrontierConn(
		serviceconnections.WithConnectTimeout(connectTimeout),
		serviceconnections.WithHost(viper.GetString("frontier-host")),
		serviceconnections.WithPort(viper.GetString("frontier-port")),
	)
	if err := frontierConn.Connect(); err != nil {
		log.Fatalf("Could not connect to frontier: %v", err)
	}
	robotsEvaluatorConn := serviceconnections.NewRobotsEvaluatorConn(
		serviceconnections.WithConnectTimeout(connectTimeout),
		serviceconnections.WithHost(viper.GetString("robots-evaluator-host")),
		serviceconnections.WithPort(viper.GetString("robots-evaluator-port")),
	)
	if err := robotsEvaluatorConn.Connect(); err != nil {
		log.Fatalf("Could not connect to robots evaluator: %v", err)
	}

	db := database.NewConnection(
		viper.GetString("db-host"),
		viper.GetInt("db-port"),
		viper.GetString("db-user"),
		viper.GetString("db-password"),
		viper.GetString("db-name"),
	)
	if err := db.Connect(); err != nil {
		log.Fatalf("Could not connect to database: %v", err)
	}

	configCache := database.NewDbAdapter(db, 5*time.Minute)

	bc := controller.New(
		controller.WithListenInterface(viper.GetString("interface")),
		controller.WithListenPort(viper.GetInt("port")),
		controller.WithContentWriterConn(contentWriterConn),
		controller.WithFrontierConn(frontierConn),
		controller.WithRobotsEvaluatorConn(robotsEvaluatorConn),
		controller.WithMaxConcurrentSessions(viper.GetInt("proxy-count")),
		controller.WithSessionOptions(
			session.WithBrowserHost(viper.GetString("browser-host")),
			session.WithBrowserPort(viper.GetInt("browser-port")),
			session.WithProxyHost(viper.GetString("proxy-host")),
			session.WithProxyPort(viper.GetInt("proxy-port")),
			session.WithDbAdapter(configCache),
		),
	)
	go bc.Start()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	func() {
		for sig := range c {
			// sig is a ^C, handle it
			log.Debugf("Got signal: %v", sig)
			bc.Stop()
			return
		}
	}()
}
