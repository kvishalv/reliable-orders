FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy source code (including go.mod)
COPY . .

# Download dependencies and create go.sum
RUN go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o payment-service ./cmd

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/payment-service .

EXPOSE 8081

CMD ["./payment-service"]
