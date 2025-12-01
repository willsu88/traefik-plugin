# traefik-free-tier-plugin

A Traefik middleware plugin that rate limits requests using `free:free` Basic Auth credentials, per client IP.

## Features

- Only rate limits requests with `Authorization: Basic <free:free>`
- Other requests pass through without rate limiting
- Per-IP rate limiting using token bucket algorithm
- Configurable rate and burst settings

## Configuration

```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: free-tier-limiter
spec:
  plugin:
    free-tier-limiter:
      rate: 10   # requests per second
      burst: 20  # max burst
```

## How it works

1. Checks if the request has `Authorization: Basic ZnJlZTpmcmVl` header (free:free)
2. If not, passes the request through without any rate limiting
3. If yes, applies per-IP rate limiting using a token bucket algorithm
4. Returns 429 Too Many Requests when rate limit is exceeded

## Testing

```bash
go test -v
```
