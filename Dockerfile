FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod main.go reconcile.go schema.sql ./
RUN CGO_ENABLED=0 go build -o /emit .
FROM alpine:3.21
RUN apk add --no-cache postgresql17-client
COPY --from=build /emit /usr/local/bin/emit
ENTRYPOINT ["emit"]
