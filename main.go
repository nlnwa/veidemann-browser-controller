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
	"github.com/nlnwa/veidemann-browser-controller/pkg/harvester"
	"github.com/nlnwa/veidemann-browser-controller/pkg/logger"
	"github.com/nlnwa/veidemann-browser-controller/pkg/metrics"
	"github.com/nlnwa/veidemann-browser-controller/pkg/robotsevaluator"
	"github.com/nlnwa/veidemann-browser-controller/pkg/screenshotwriter"
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
	pflag.Int("proxy-count", 10, "max number of simultaneous sessions. Must match RecorderProxy's proxy-count setting.")

	pflag.String("browser-host", "localhost", "Browser host")
	pflag.Int("browser-port", 3000, "Browser port")

	pflag.String("proxy-host", "localhost", "Recorder proxy host")
	pflag.Int("proxy-port", 9900, "Recorder proxy port")

	pflag.String("content-writer-host", "veidemann-contentwriter", "Content writer host")
	pflag.Int("content-writer-port", 8082, "Content writer port")

	pflag.String("frontier-host", "veidemann-frontier", "Frontier host")
	pflag.Int("frontier-port", 7700, "Frontier port")

	pflag.String("robots-evaluator-host", "veidemann-robotsevaluator-service", "Robots evaluator host")
	pflag.Int("robots-evaluator-port", 7053, "Robots evaluator port")
	pflag.Duration("connect-timeout", 10*time.Second, "Timeout used for connecting to GRPC services")

	pflag.String("db-host", "rethinkdb-proxy", "DB host")
	pflag.Int("db-port", 28015, "DB port")
	pflag.String("db-name", "veidemann", "DB name")
	pflag.String("db-user", "", "DB user name")
	pflag.String("db-password", "", "DB password")

	pflag.String("metrics-interface", "", "Interface for exposing metrics. Empty means all interfaces")
	pflag.Int("metrics-port", 9153, "Port for exposing metrics")
	pflag.String("metrics-path", "/metrics", "Path for exposing metrics")

	pflag.String("log-level", "info", "log level, available levels are panic, fatal, error, warn, info, debug and trace")
	pflag.String("log-formatter", "logfmt", "log formatter, available values are logfmt and json")
	pflag.Bool("log-method", false, "log method names")

	pflag.Parse()

	viper.SetDefault("ContentDir", "content")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		log.Errorf("Could not parse flags: %s", err)
		return
	}

	if viper.GetBool("help") {
		pflag.Usage()
		return
	}

	// init logger
	if err := logger.InitLog(
		viper.GetString("log-level"),
		viper.GetString("log-formatter"),
		viper.GetBool("log-method"),
	); err != nil {
		log.Errorf("Could not initialize logs: %v", err)
		return
	}

	log.Infof("Browser Controller starting...")
	defer func() {
		log.Infof("Browser Controller stopped")
	}()

	// setup tracing
	tracer, closer := tracing.Init("Browser Controller")
	if tracer != nil {
		opentracing.SetGlobalTracer(tracer)
		defer closer.Close()
	}

	metricsServer := metrics.NewServer(viper.GetString("metrics-interface"), viper.GetInt("metrics-port"), viper.GetString("metrics-path"))

	connectTimeout := viper.GetDuration("connect-timeout")

	screenshotwriter := screenshotwriter.New(
		serviceconnections.WithConnectTimeout(connectTimeout),
		serviceconnections.WithHost(viper.GetString("content-writer-host")),
		serviceconnections.WithPort(viper.GetInt("content-writer-port")),
	)
	if err := screenshotwriter.Connect(); err != nil {
		log.WithError(err).Error()
		return
	}
	defer screenshotwriter.Close()

	harvester := harvester.New(
		serviceconnections.WithConnectTimeout(connectTimeout),
		serviceconnections.WithHost(viper.GetString("frontier-host")),
		serviceconnections.WithPort(viper.GetInt("frontier-port")),
	)
	if err := harvester.Connect(); err != nil {
		log.WithError(err).Error()
		return
	}
	defer harvester.Close()

	robotsEvaluator := robotsevaluator.New(
		serviceconnections.WithConnectTimeout(connectTimeout),
		serviceconnections.WithHost(viper.GetString("robots-evaluator-host")),
		serviceconnections.WithPort(viper.GetInt("robots-evaluator-port")),
	)
	if err := robotsEvaluator.Connect(); err != nil {
		log.WithError(err).Error()
		return
	}
	defer robotsEvaluator.Close()

	db := database.NewConnection(
		viper.GetString("db-host"),
		viper.GetInt("db-port"),
		viper.GetString("db-user"),
		viper.GetString("db-password"),
		viper.GetString("db-name"),
	)
	if err := db.Connect(); err != nil {
		log.WithError(err).Error()
		return
	}
	defer db.Close()

	configCache := database.NewDbAdapter(db, 5*time.Minute)

	browserController := controller.New(
		controller.WithListenInterface(viper.GetString("interface")),
		controller.WithListenPort(viper.GetInt("port")),
		controller.WithMaxConcurrentSessions(viper.GetInt("proxy-count")),
		controller.WithHarvester(harvester),
		controller.WithRobotsEvaluator(robotsEvaluator),
		controller.WithSessionOptions(
			session.WithScreenshotWriter(screenshotwriter),
			session.WithBrowserHost(viper.GetString("browser-host")),
			session.WithBrowserPort(viper.GetInt("browser-port")),
			session.WithProxyHost(viper.GetString("proxy-host")),
			session.WithProxyPort(viper.GetInt("proxy-port")),
			session.WithDbAdapter(configCache),
		),
	)

	errc := make(chan error, 1)

	go func() { errc <- metricsServer.Start() }()
	defer metricsServer.Close()

	go func() {
		signals := make(chan os.Signal)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

		select {
		case err := <-errc:
			log.WithError(err).Error("Metrics server failed")
			browserController.Shutdown()
		case sig := <-signals:
			log.Debugf("Received signal: %s", sig)
			browserController.Shutdown()
		}
	}()

	err := browserController.Run()
	if err != nil {
		log.WithError(err).Error()
	}
}
