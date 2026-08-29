# CausaLens demo-ledger service (Member 1 owned path).
# Build context is the repository root:
#   docker build -t causalens/demo-ledger:dev -f cmd/demo-ledger/Dockerfile .
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/demo-ledger ./cmd/demo-ledger

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && \
    addgroup -S causalens && adduser -S -G causalens causalens
COPY --from=build /out/demo-ledger /usr/local/bin/demo-ledger
USER causalens
EXPOSE 8083
ENTRYPOINT ["demo-ledger"]
