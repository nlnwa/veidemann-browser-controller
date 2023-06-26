![Docker](https://github.com/nlnwa/veidemann-browser-controller/workflows/Docker/badge.svg)

# veidemann-browser-controller

## Test

    go test ./...

## Run integration test

The integration test is useful to validate compatibility between the `browserless` container image and the `chromedp` library.

### Using docker

    go test --tags=integration -run TestSession_Fetch ./server

### Using podman

> Note that you have to manually clean up containers after every run with using podman as container provider because the container reaper (ryuk) is disabled.

    # Setup podman socket
    systemctl --user start podman

    # Run integration test
    DOCKER_HOST="unix:///run/user/$UID/podman/podman.sock" go test -run TestSession_Fetch --tags=integration ./server -provider=podman
