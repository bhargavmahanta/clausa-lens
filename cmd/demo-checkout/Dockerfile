# CausaLens demo-checkout service (Member 1 owned path).
# Build context is the repository root:
#   docker build -t causalens/demo-checkout:dev -f cmd/demo-checkout/Dockerfile .
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/demo-checkout ./cmd/demo-checkout

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && \
    addgroup -S causalens && adduser -S -G causalens causalens
COPY --from=build /out/demo-checkout /usr/local/bin/demo-checkout
USER causalens
EXPOSE 8081
ENTRYPOINT ["demo-checkout"]
