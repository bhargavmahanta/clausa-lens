# CausaLens Replay Worker (Member 2 owned path).
# Build context is the repository root (see deploy/compose.yaml). The worker is
# driven per run via -run-id; it selects the System Pack from PACK_IMPL using
# the same pack-agnostic registry as the Core API.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/replay-worker ./cmd/replay-worker

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/replay-worker /usr/local/bin/replay-worker
ENTRYPOINT ["replay-worker"]
