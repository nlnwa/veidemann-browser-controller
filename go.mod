module github.com/nlnwa/veidemann-browser-controller

go 1.16

require (
	github.com/bits-and-blooms/bitset v1.4.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/chromedp/cdproto v0.0.0-20201009231348-1c6a710e77de
	github.com/chromedp/chromedp v0.5.5
	github.com/docker/go-connections v0.4.0
	github.com/google/uuid v1.3.0
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mailru/easyjson v0.7.7
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/nlnwa/veidemann-api/go v0.0.0-20220110104816-ea13deeb9671
	github.com/nlnwa/veidemann-log-service v0.2.0
	github.com/nlnwa/veidemann-recorderproxy v0.3.0
	github.com/nlnwa/whatwg-url v0.1.2
	github.com/opentracing/opentracing-go v1.2.0
	github.com/pelletier/go-toml/v2 v2.0.6 // indirect
	github.com/prometheus/client_golang v1.14.0
	github.com/prometheus/common v0.39.0
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/rs/zerolog v1.28.0
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/spf13/afero v1.9.3 // indirect
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.14.0
	github.com/testcontainers/testcontainers-go v0.10.0
	github.com/uber/jaeger-client-go v2.30.0+incompatible
	golang.org/x/crypto v0.4.0 // indirect
	google.golang.org/genproto v0.0.0-20221227171554-f9683d7f8bef // indirect
	google.golang.org/grpc v1.51.0
	google.golang.org/protobuf v1.28.1
	gopkg.in/rethinkdb/rethinkdb-go.v6 v6.2.2
)

//replace github.com/getlantern/proxy => ../getlantern-proxy

//replace github.com/nlnwa/veidemann-recorderproxy => ../veidemann-recorderproxy

replace github.com/getlantern/proxy => github.com/nlnwa/getlantern-proxy v0.0.0-20200424070054-d94d64dd7b79
