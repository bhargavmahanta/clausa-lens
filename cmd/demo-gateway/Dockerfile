# CausaLens demo-gateway service (Member 1 owned path).
# Build context is the repository root:
#   docker build -t causalens/demo-gateway:dev -f cmd/demo-gateway/Dockerfile .
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/demo-gateway ./cmd/demo-gateway

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && \
    addgroup -S causalens && adduser -S -G causalens causalens
COPY --from=build /out/demo-gateway /usr/local/bin/demo-gateway
USER causalens
EXPOSE 8080
ENTRYPOINT ["demo-gateway"]
