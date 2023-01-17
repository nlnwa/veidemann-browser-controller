module github.com/nlnwa/veidemann-browser-controller

go 1.16

require (
	github.com/chromedp/cdproto v0.0.0-20230109101555-6b041c6303cc
	github.com/chromedp/chromedp v0.8.7
	github.com/docker/go-connections v0.4.0
	github.com/google/uuid v1.3.0
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645
	github.com/mailru/easyjson v0.7.7
	github.com/nlnwa/veidemann-api/go v0.0.0-20220110104816-ea13deeb9671
	github.com/nlnwa/veidemann-log-service v0.2.0
	github.com/nlnwa/veidemann-recorderproxy v0.4.0
	github.com/nlnwa/whatwg-url v0.1.2
	github.com/opentracing/opentracing-go v1.2.0
	github.com/prometheus/client_golang v1.14.0
	github.com/prometheus/common v0.39.0
	github.com/rs/zerolog v1.28.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.14.0
	github.com/testcontainers/testcontainers-go v0.17.0
	github.com/uber/jaeger-client-go v2.30.0+incompatible
	google.golang.org/grpc v1.52.0
	google.golang.org/protobuf v1.28.1
	gopkg.in/rethinkdb/rethinkdb-go.v6 v6.2.2
)

require (
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/bits-and-blooms/bitset v1.4.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/containerd/containerd v1.6.15 // indirect
	github.com/docker/docker v20.10.22+incompatible // indirect
	github.com/getlantern/errors v0.0.0-20190325191628-abdb3e3e36f7
	github.com/getlantern/mitm v0.0.0-20180205214248-4ce456bae650
	github.com/getlantern/proxy v0.0.0-20190225163220-31d1cc06ed3d
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/moby/term v0.0.0-20221205130635-1aeaba878587 // indirect
	github.com/opencontainers/runc v1.1.4 // indirect
	github.com/pelletier/go-toml/v2 v2.0.6 // indirect
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/sirupsen/logrus v1.9.0
	github.com/spf13/afero v1.9.3 // indirect
	github.com/subosito/gotenv v1.4.2 // indirect
	golang.org/x/crypto v0.5.0 // indirect
	golang.org/x/tools v0.5.0 // indirect
	google.golang.org/genproto v0.0.0-20230113154510-dbe35b8444a5 // indirect
)

replace github.com/docker/docker => github.com/docker/docker v20.10.3-0.20221013203545-33ab36d6b304+incompatible // 22.06 branch

replace github.com/getlantern/proxy => github.com/nlnwa/getlantern-proxy v0.0.0-20200424070054-d94d64dd7b79
