module github.com/nlnwa/veidemann-browser-controller

go 1.16

require (
	github.com/HdrHistogram/hdrhistogram-go v1.1.2 // indirect
	github.com/chromedp/cdproto v0.0.0-20200709115526-d1f6fc58448b
	github.com/chromedp/chromedp v0.5.4-0.20200417165948-9fff3ea3e94b
	github.com/docker/go-connections v0.4.0
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/google/uuid v1.2.0
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645
	github.com/mailru/easyjson v0.7.2
	github.com/mitchellh/mapstructure v1.3.3 // indirect
	github.com/nlnwa/veidemann-api/go v0.0.0-20210413093311-7ff38e848604
	github.com/nlnwa/veidemann-log-service v0.1.6
	github.com/nlnwa/veidemann-recorderproxy v0.3.0
	github.com/nlnwa/whatwg-url v0.1.0
	github.com/opentracing/opentracing-go v1.2.0
	github.com/pelletier/go-toml v1.8.0 // indirect
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/common v0.10.0
	github.com/rs/zerolog v1.25.0
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/afero v1.3.3 // indirect
	github.com/spf13/cast v1.3.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	github.com/testcontainers/testcontainers-go v0.10.0
	github.com/uber/jaeger-client-go v2.27.0+incompatible
	go.uber.org/atomic v1.6.0 // indirect
	google.golang.org/grpc v1.33.2
	google.golang.org/protobuf v1.26.0
	gopkg.in/ini.v1 v1.57.0 // indirect
	gopkg.in/rethinkdb/rethinkdb-go.v6 v6.2.1
)

//replace github.com/getlantern/proxy => ../getlantern-proxy

//replace github.com/nlnwa/veidemann-recorderproxy => ../veidemann-recorderproxy

replace github.com/getlantern/proxy => github.com/nlnwa/getlantern-proxy v0.0.0-20200424070054-d94d64dd7b79
