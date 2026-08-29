# CausaLens demo-payment service (Member 1 owned path).
# Build context is the repository root:
#   docker build -t causalens/demo-payment:dev -f cmd/demo-payment/Dockerfile .
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/demo-payment ./cmd/demo-payment

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && \
    addgroup -S causalens && adduser -S -G causalens causalens
COPY --from=build /out/demo-payment /usr/local/bin/demo-payment
USER causalens
EXPOSE 8082
ENTRYPOINT ["demo-payment"]
