module github.com/nlnwa/veidemann-browser-controller

go 1.16

require (
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/bits-and-blooms/bitset v1.4.0 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/chromedp/cdproto v0.0.0-20201009231348-1c6a710e77de
	github.com/chromedp/chromedp v0.5.5
	github.com/containerd/containerd v1.7.2 // indirect
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/docker/docker v24.0.2+incompatible // indirect
	github.com/docker/go-connections v0.4.0
	github.com/google/uuid v1.3.0
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645
	github.com/klauspost/compress v1.16.6 // indirect
	github.com/mailru/easyjson v0.7.7
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/nlnwa/veidemann-api/go v0.0.0-20220110104816-ea13deeb9671
	github.com/nlnwa/veidemann-log-service v0.2.0
	github.com/nlnwa/veidemann-recorderproxy v0.3.0
	github.com/nlnwa/whatwg-url v0.1.2
	github.com/opencontainers/image-spec v1.1.0-rc3 // indirect
	github.com/opencontainers/runc v1.1.7 // indirect
	github.com/opentracing/opentracing-go v1.2.0
	github.com/pelletier/go-toml/v2 v2.0.6 // indirect
	github.com/prometheus/client_golang v1.14.0
	github.com/prometheus/common v0.39.0
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/rs/zerolog v1.28.0
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/afero v1.9.3 // indirect
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.14.0
	github.com/testcontainers/testcontainers-go v0.19.0
	github.com/uber/jaeger-client-go v2.30.0+incompatible
	golang.org/x/tools v0.10.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230530153820-e85fd2cbaebc // indirect
	google.golang.org/grpc v1.56.3
	google.golang.org/protobuf v1.31.0
	gopkg.in/rethinkdb/rethinkdb-go.v6 v6.2.2
	gotest.tools/v3 v3.4.0 // indirect
)

//replace github.com/getlantern/proxy => ../getlantern-proxy

//replace github.com/nlnwa/veidemann-recorderproxy => ../veidemann-recorderproxy

replace github.com/getlantern/proxy => github.com/nlnwa/getlantern-proxy v0.0.0-20200424070054-d94d64dd7b79
