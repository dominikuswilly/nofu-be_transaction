# Build Stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install git if needed for dependencies
# RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the binary
# CGO_ENABLED=0 is important for alpine standard binaries
RUN CGO_ENABLED=0 GOOS=linux go build -o transaction-service cmd/api/main.go

# Run Stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates (useful if making external HTTPS calls)
RUN apk --no-cache add ca-certificates

COPY --from=builder /app/transaction-service .

# Expose the application port
EXPOSE 8080

# Run the application
CMD ["./transaction-service"]
