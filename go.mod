module github.com/nlnwa/veidemann-browser-controller

go 1.13

require (
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/chromedp/cdproto v0.0.0-20200209033844-7e00b02ea7d2
	github.com/chromedp/chromedp v0.5.4-0.20200417165948-9fff3ea3e94b
	github.com/containerd/continuity v0.0.0-20200228182428-0f16d7a0959c // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/golang/protobuf v1.4.2
	github.com/gotestyourself/gotestyourself v2.2.0+incompatible // indirect
	github.com/lib/pq v1.3.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/nlnwa/veidemann-api-go v1.0.0-beta16
	github.com/nlnwa/veidemann-recorderproxy v0.2.6
	github.com/nlnwa/whatwg-url v0.0.0-20200322073154-8c2d737699d5
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/opentracing/opentracing-go v1.1.0
	github.com/ory/dockertest v3.3.5+incompatible
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.5.1
	github.com/sirupsen/logrus v1.5.0
	github.com/spf13/pflag v1.0.3
	github.com/spf13/viper v1.3.2
	github.com/uber/jaeger-client-go v2.17.0+incompatible
	google.golang.org/grpc v1.29.1
	google.golang.org/protobuf v1.24.0
	gopkg.in/rethinkdb/rethinkdb-go.v6 v6.2.1
	gotest.tools v2.2.0+incompatible // indirect
)

//replace github.com/getlantern/proxy => ../getlantern-proxy

//replace github.com/nlnwa/veidemann-recorderproxy => ../veidemann-recorderproxy

replace github.com/getlantern/proxy => github.com/nlnwa/getlantern-proxy v0.0.0-20200424070054-d94d64dd7b79
