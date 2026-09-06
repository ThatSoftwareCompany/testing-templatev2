FROM golang:1.26.7-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
	&& CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/auth ./cmd/auth \
	&& CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM golang:1.26.7-alpine AS development

WORKDIR /app
COPY --from=build /go/pkg/mod /go/pkg/mod
COPY . .
CMD ["go", "run", "./cmd/api"]

FROM alpine:3.22 AS production

RUN apk upgrade --no-cache \
	&& addgroup -S app \
	&& adduser -S -G app app
WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /out/auth /app/auth
COPY --from=build /out/migrate /app/migrate
COPY --from=build /src/migrations /app/migrations
USER app
EXPOSE 8080
ENTRYPOINT ["/app/api"]
