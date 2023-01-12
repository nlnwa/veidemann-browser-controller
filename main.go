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
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/nlnwa/veidemann-browser-controller/controller"
	"github.com/nlnwa/veidemann-browser-controller/database"
	"github.com/nlnwa/veidemann-browser-controller/frontier"
	"github.com/nlnwa/veidemann-browser-controller/logger"
	"github.com/nlnwa/veidemann-browser-controller/logwriter"
	"github.com/nlnwa/veidemann-browser-controller/metrics"
	"github.com/nlnwa/veidemann-browser-controller/robotsevaluator"
	"github.com/nlnwa/veidemann-browser-controller/screenshotwriter"
	"github.com/nlnwa/veidemann-browser-controller/serviceconnections"
	"github.com/nlnwa/veidemann-browser-controller/session"
	"github.com/nlnwa/veidemann-browser-controller/tracing"
	"github.com/opentracing/opentracing-go"
	"github.com/rs/zerolog/log"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
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

	pflag.String("log-service-host", "veidemann-log-service", "Log service host")
	pflag.Int("log-service-port", 8080, "Log service port")

	pflag.String("robots-evaluator-host", "veidemann-robotsevaluator-service", "Robots evaluator host")
	pflag.Int("robots-evaluator-port", 7053, "Robots evaluator port")
	pflag.Duration("connect-timeout", 10*time.Second, "Timeout used for connecting to GRPC services")

	pflag.String("db-host", "rethinkdb-proxy", "DB host")
	pflag.Int("db-port", 28015, "DB port")
	pflag.String("db-name", "veidemann", "DB name")
	pflag.String("db-user", "", "Database username")
	pflag.String("db-password", "", "Database password")
	pflag.Duration("db-query-timeout", 1*time.Minute, "Database query timeout")
	pflag.Int("db-max-retries", 5, "Max retries when database query fails")
	pflag.Int("db-max-open-conn", 10, "Max open database connections")
	pflag.Bool("db-use-opentracing", false, "Use opentracing for database queries")
	pflag.Duration("db-cache-ttl", 5*time.Minute, "How long to cache results from database")

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
		log.Fatal().Err(err).Msg("Failed to parse flags")
	}

	if viper.GetBool("help") {
		pflag.Usage()
		return
	}

	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				log.Fatal().Err(err).Msg("Fatal error")
			}
		}
	}()

	// init logger
	logger.InitLog(
		viper.GetString("log-level"),
		viper.GetString("log-formatter"),
		viper.GetBool("log-method"),
	)

	log.Info().Msg("Browser Controller starting...")
	defer func() {
		log.Info().Msg("Browser Controller stopped")
	}()

	// setup tracing
	if tracer, closer, err := tracing.Init("Browser Controller"); err != nil {
		log.Warn().Err(err).Msg("Failed to initialize tracing")
	} else {
		defer closer.Close()
		opentracing.SetGlobalTracer(tracer)
	}

	connectTimeout := viper.GetDuration("connect-timeout")

	screenshotWriter := screenshotwriter.New(
		serviceconnections.WithConnectTimeout(connectTimeout),
		serviceconnections.WithHost(viper.GetString("content-writer-host")),
		serviceconnections.WithPort(viper.GetInt("content-writer-port")),
	)
	if err := screenshotWriter.Connect(); err != nil {
		panic(err)
	}
	defer screenshotWriter.Close()

	frontier := frontier.New(
		serviceconnections.WithConnectTimeout(connectTimeout),
		serviceconnections.WithHost(viper.GetString("frontier-host")),
		serviceconnections.WithPort(viper.GetInt("frontier-port")),
		serviceconnections.WithDialOptions(
			grpc.WithUnaryInterceptor(otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer())),
			grpc.WithStreamInterceptor(otgrpc.OpenTracingStreamClientInterceptor(opentracing.GlobalTracer())),
		),
	)
	if err := frontier.Connect(); err != nil {
		panic(err)
	}
	defer frontier.Close()

	robotsEvaluator := robotsevaluator.New(
		serviceconnections.WithConnectTimeout(connectTimeout),
		serviceconnections.WithHost(viper.GetString("robots-evaluator-host")),
		serviceconnections.WithPort(viper.GetInt("robots-evaluator-port")),
	)
	if err := robotsEvaluator.Connect(); err != nil {
		panic(err)
	}
	defer robotsEvaluator.Close()

	logWriter := logwriter.New(
		serviceconnections.WithConnectTimeout(connectTimeout),
		serviceconnections.WithHost(viper.GetString("log-service-host")),
		serviceconnections.WithPort(viper.GetInt("log-service-port")),
	)
	if err := logWriter.Connect(); err != nil {
		panic(err)
	}
	defer logWriter.Close()

	db := database.NewRethinkDbConnection(
		database.Options{
			Address:            fmt.Sprintf("%s:%d", viper.GetString("db-host"), viper.GetInt("db-port")),
			Username:           viper.GetString("db-user"),
			Password:           viper.GetString("db-password"),
			Database:           viper.GetString("db-name"),
			QueryTimeout:       viper.GetDuration("db-query-timeout"),
			MaxOpenConnections: viper.GetInt("db-max-open-conn"),
			MaxRetries:         viper.GetInt("db-max-retries"),
			UseOpenTracing:     viper.GetBool("db-use-opentracing"),
		},
	)
	if err := db.Connect(); err != nil {
		panic(err)
	}
	defer db.Close()

	configCache := database.NewConfigCache(db, viper.GetDuration("db-cache-ttl"))

	browserController := controller.New(
		controller.WithListenInterface(viper.GetString("interface")),
		controller.WithListenPort(viper.GetInt("port")),
		controller.WithMaxConcurrentSessions(viper.GetInt("proxy-count")),
		controller.WithFrontier(frontier),
		controller.WithRobotsEvaluator(robotsEvaluator),
		controller.WithLogWriter(logWriter),
		controller.WithSessionOptions(
			session.WithLogWriter(logWriter),
			session.WithScreenshotWriter(screenshotWriter),
			session.WithBrowserHost(viper.GetString("browser-host")),
			session.WithBrowserPort(viper.GetInt("browser-port")),
			session.WithProxyHost(viper.GetString("proxy-host")),
			session.WithProxyPort(viper.GetInt("proxy-port")),
			session.WithConfigCache(configCache),
		),
	)

	metricsServer := metrics.NewServer(viper.GetString("metrics-interface"), viper.GetInt("metrics-port"), viper.GetString("metrics-path"))
	go func() {
		if err := metricsServer.Start(); err != nil {
			log.Error().Err(err).Msg("Metrics server failed")
			browserController.Shutdown()
		}
	}()
	defer metricsServer.Close()

	go func() {
		signals := make(chan os.Signal, 2)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		sig := <-signals
		log.Debug().Str("signal", sig.String()).Msg("Received signal")
		browserController.Shutdown()
	}()

	if err := browserController.Run(); err != nil {
		panic(err)
	}
}
