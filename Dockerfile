FROM golang:1.14 as build

WORKDIR /build

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

# -trimpath remove file system paths from executable
# -ldflags arguments passed to go tool link:
#   -s disable symbol table
#   -w disable DWARF generation
RUN go build -trimpath -ldflags "-s -w" .

FROM gcr.io/distroless/base
COPY --from=build /build/veidemann-browser-controller /

# api server
EXPOSE 8080/tcp
# prometheus metrics server
EXPOSE 9153/tcp

ENTRYPOINT ["/veidemann-browser-controller"]
