FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /tlaloc-backend .

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /tlaloc-backend /usr/local/bin/tlaloc-backend
ENTRYPOINT ["tlaloc-backend"]
CMD ["-config", "/etc/tlaloc/config.yaml"]
