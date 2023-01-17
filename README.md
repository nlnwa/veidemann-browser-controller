![Docker](https://github.com/nlnwa/veidemann-browser-controller/workflows/Docker/badge.svg)

# veidemann-browser-controller

## Test

### Run test to validate compatibility between browserless and chromedp:

```bash
# Setup podman socket on fedora
systemctl --user start podman

# Run integration test
DOCKER_HOST="unix:///run/user/$UID/podman/podman.sock" go test --tags=integration ./server
```
