FROM golang:1.26-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o pc-price-server .

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /build/pc-price-server .
COPY --from=builder /build/templates ./templates
COPY --from=builder /build/static ./static

ENV HOST=0.0.0.0
ENV PORT=8090

EXPOSE 8090

CMD ["./pc-price-server"]
