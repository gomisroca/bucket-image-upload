# bucket-image-upload

A standalone Go microservice that accepts image uploads, generates resized
thumbnails **concurrently**, and stores everything for retrieval over HTTP.
Built so any other application - a React frontend, a mobile backend, a CMS,
whatever - can "plug and play" with it over plain HTTP/JSON, without needing
to know anything about Go.

## Why this architecture

- **Storage is an interface, not a filesystem call.** `internal/storage`
  defines a `Storage` interface. `LocalStorage` and `S3Storage` are two
  interchangeable implementations of it - `main.go` picks one based on a
  single env var (`STORAGE_BACKEND`). Nothing in `internal/handlers` knows
  or cares which one is active.
- **Every URL the client sees is `/files/{key}`, never a raw storage path.**
  The upload response never exposes a local disk path or a presigned S3
  URL directly. Instead it points at this service's own `/files/{key}`
  endpoint, which resolves the real location _at request time_. That
  matters because S3/R2 presigned URLs expire - baking one into the upload
  response would mean links going dead later. Resolving fresh on every
  request means they never do.
- **Concurrent thumbnail generation.** Each requested thumbnail size is
  resized and encoded in its own goroutine (`internal/imaging/resize.go`,
  `GenerateThumbnails`). Generating several thumbnail sizes costs barely more
  wall-clock time than generating one - this is the concurrency story that's
  genuinely awkward to get right in most other languages but is a few lines
  in Go.
- **Image processing has zero dependencies; cloud storage uses the real AWS
  SDK.** Decoding/resizing/encoding are pure standard library - no
  supply-chain surface area for the part that's easy to get right. Talking
  to S3/R2 uses the official `aws-sdk-go-v2`, because hand-rolling AWS
  request signing (SigV4) is the kind of thing that's easy to get subtly
  wrong and painful to debug without a live bucket to test against.
- **Single static binary.** `go build` produces one binary with no runtime
  dependency. The included `Dockerfile` uses a multi-stage build that copies
  just that binary into `alpine`, so the final image has no language
  runtime, no package manager, almost no attack surface.

## Running it

```bash
go run .
# or
go build -o server . && ./server
```

Then open `http://localhost:8080` for a small built-in test page (drag and
drop an image, see it upload, resize, and display), or call the API
directly:

```bash
curl -X POST -F "image=@photo.jpg" http://localhost:8080/upload
```

### Config (env vars)

**Always available:**

| Variable           | Default    | Description                                                                                                                                                                                                                                 |
| ------------------ | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PORT`             | `8080`     | HTTP port                                                                                                                                                                                                                                   |
| `MAX_UPLOAD_BYTES` | `10485760` | Max request body size (bytes, 10 MB)                                                                                                                                                                                                        |
| `API_KEY`          | _(empty)_  | If set, required via `X-API-Key` header on `POST /upload`. Leave empty for local dev - the service logs a warning at startup if so. `GET /files/{key}` is never protected by this (browsers can't send custom headers on `<img>` requests). |
| `STORAGE_BACKEND`  | `local`    | `local` or `s3`                                                                                                                                                                                                                             |

**`STORAGE_BACKEND=local`:**

| Variable     | Default     | Description                    |
| ------------ | ----------- | ------------------------------ |
| `UPLOAD_DIR` | `./uploads` | Where files are stored on disk |

**`STORAGE_BACKEND=s3`** (works for AWS S3, Cloudflare R2, or any
S3-compatible API):

| Variable                 | Required? | Description                                                                                                                                   |
| ------------------------ | --------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `S3_BUCKET`              | yes       | Bucket name                                                                                                                                   |
| `S3_REGION`              | no        | AWS region (e.g. `us-east-1`). Defaults to `auto` (what R2 expects)                                                                           |
| `S3_ENDPOINT`            | R2 only   | Custom endpoint. Leave empty for AWS S3. For R2: `https://<account_id>.r2.cloudflarestorage.com`                                              |
| `S3_ACCESS_KEY_ID`       | yes       | Access key                                                                                                                                    |
| `S3_SECRET_ACCESS_KEY`   | yes       | Secret key                                                                                                                                    |
| `S3_PUBLIC_BASE_URL`     | no        | Public bucket/CDN URL (e.g. R2 public bucket URL, or CloudFront domain). If set, files are served directly here instead of via presigned URLs |
| `S3_PRESIGN_TTL_SECONDS` | no        | Default `3600`. Only used when `S3_PUBLIC_BASE_URL` is empty                                                                                  |

**Rate limiting** (optional - talks to
[`ratelimiter-service`](../ratelimiter-service)). If `RATELIMITER_URL` is
unset, no rate-limit check happens here at all; the service relies entirely
on whatever's calling it to enforce limits upstream:

| Variable                | Default   | Description                                                                                                                                                                    |
| ----------------------- | --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `RATELIMITER_URL`       | _(empty)_ | Base URL of the rate-limiter service, e.g. `http://localhost:8081`. Empty disables the check entirely.                                                                         |
| `RATELIMITER_API_KEY`   | _(empty)_ | Sent as `X-API-Key` to the rate limiter, if it requires one                                                                                                                    |
| `RATELIMITER_FAIL_OPEN` | `true`    | If the rate limiter is unreachable or errors: `true` lets the upload through anyway (an outage in the limiter shouldn't take down uploads); `false` rejects with `503` instead |

When this is enabled, callers should pass an `X-Client-Key` header
identifying the _real end user_ - not just the immediate caller. This
matters most when a backend (e.g. a Next.js API route) is forwarding the
request server-to-server: without `X-Client-Key`, every one of your users
would get lumped into a single bucket keyed on your backend's own IP. With
it, each user gets their own limit:

```js
fetch(`${UPLOADER_URL}/upload`, {
  method: "POST",
  headers: {
    "X-API-Key": process.env.UPLOADER_API_KEY,
    "X-Client-Key": userId, // the actual end user, not the caller
  },
  body: form,
});
```

If no `X-Client-Key` is sent, the uploader falls back to the connecting
IP - coarser (one bucket per calling service, not per end user), but still
real protection against runaway load.

**Cloudflare R2 example** (private bucket, accessed via presigned URLs):

```bash
export STORAGE_BACKEND=s3
export S3_BUCKET=my-uploads
export S3_REGION=auto
export S3_ENDPOINT=https://<your-account-id>.r2.cloudflarestorage.com
export S3_ACCESS_KEY_ID=<from R2 dashboard → Manage R2 API Tokens>
export S3_SECRET_ACCESS_KEY=<same place>
go run .
```

**AWS S3 example** (public bucket via CDN):

```bash
export STORAGE_BACKEND=s3
export S3_BUCKET=my-uploads
export S3_REGION=us-east-1
export S3_ACCESS_KEY_ID=...
export S3_SECRET_ACCESS_KEY=...
export S3_PUBLIC_BASE_URL=https://my-uploads.s3.us-east-1.amazonaws.com
go run .
```

### Docker

```bash
docker build -t bucket-image-upload .
docker run -p 8080:8080 \
  -e API_KEY=your-secret-key \
  -e STORAGE_BACKEND=s3 \
  -e S3_BUCKET=my-uploads \
  -e S3_REGION=auto \
  -e S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com \
  -e S3_ACCESS_KEY_ID=... \
  -e S3_SECRET_ACCESS_KEY=... \
  -e RATELIMITER_URL=http://ratelimiter:8080 \
  -e RATELIMITER_API_KEY=your-ratelimiter-key \
  bucket-image-upload
```

## API

### `POST /upload`

Multipart form upload. Field name must be `image`. Accepts JPEG and PNG.

**Response `201 Created`:**

```json
{
  "id": "25ff8abd42612b31",
  "original": "/files/25ff8abd42612b31_original.png",
  "thumbnails": {
    "small": "/files/25ff8abd42612b31_small.jpg",
    "medium": "/files/25ff8abd42612b31_medium.jpg"
  },
  "width": 1200,
  "height": 800,
  "contentType": "image/png"
}
```

Errors return `4xx`/`5xx` with `{"error": "..."}`. If `RATELIMITER_URL` is
configured and the caller's key is over its limit, this returns `429 Too
Many Requests` with a `Retry-After` header, before any decoding/resizing
work happens.

### `GET /files/{key}`

Redirects (`302`) to wherever the file actually lives - a local static path
or a freshly-generated presigned S3/R2 URL, depending on `STORAGE_BACKEND`.
Always call this rather than constructing a storage URL yourself; it's the
one place backend differences are resolved, and it re-resolves on every
request so links never go stale.

### `GET /uploads/{filename}`

Internal static file server, only relevant when `STORAGE_BACKEND=local`.

### `GET /health`

Liveness check - returns `{"status": "ok"}`. Standard for load balancers,
Docker healthchecks, and k8s probes.

## Calling it from another app

Any app can use this without knowing Go exists. If `API_KEY` is set, send it
via `X-API-Key` on the upload request - and do this from your own backend
(a Next.js API route, for example), not directly from the browser, so the
key never ends up in client-side JS:

```js
const form = new FormData();
form.append("image", fileInput.files[0]);

const res = await fetch("https://your-uploader.example.com/upload", {
  method: "POST",
  headers: { "X-API-Key": process.env.UPLOADER_API_KEY },
  body: form,
});
const { original, thumbnails } = await res.json();
// Save `original` / `thumbnails` URLs as-is in your DB - they're stable
// /files/{key} links on the uploader's own domain, not raw storage URLs.
```

## Possible next steps

- Add image format support (WebP, GIF) by extending `allowedTypes` and the
  decode step
- Persist upload metadata (id, dimensions, uploader) in a database instead
  of inferring everything from disk
- Add an S3 lifecycle rule (or R2 equivalent) to auto-delete originals after
  some retention window, now that storage is decoupled from local disk
- Per-key rate limit tiers (handled entirely on the `ratelimiter-service`
  side - see its README)
