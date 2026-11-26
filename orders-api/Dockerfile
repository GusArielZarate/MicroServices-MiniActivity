FROM golang:1.23-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY main.go .
RUN CGO_ENABLED=0 go build -o orders-api .

FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache sqlite ca-certificates \
    && mkdir -p /app/data

COPY --from=builder /build/orders-api /app/orders-api

ENV DB_PATH=/app/data/orders.db
ENV PORT=8002

EXPOSE 8002

CMD ["/app/orders-api"]
