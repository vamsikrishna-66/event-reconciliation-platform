FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -o /emit .
FROM alpine:3.21
COPY --from=build /emit /usr/local/bin/emit
ENTRYPOINT ["emit"]
