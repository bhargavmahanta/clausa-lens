# CausaLens Core API (Member 2 owned path).
# Build context is the repository root (see deploy/compose.yaml).
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/core-api ./cmd/core-api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/core-api /usr/local/bin/core-api
EXPOSE 8080
ENTRYPOINT ["core-api"]
