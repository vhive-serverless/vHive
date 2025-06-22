# --- Build stage ---
FROM golang:1.22.9-alpine AS builder

# Install git and CA certificates
RUN apk add --no-cache git ca-certificates

# Set the working directory inside the container
WORKDIR /build

# Copy source code
COPY . .

# Build the Go application
RUN cd ./k8s/scheduler/extender && \
    go build -o /out/scheduler-extender main.go

# --- Final stage ---
FROM alpine:3.20

# Install CA certificates (for HTTPS access)
RUN apk add --no-cache ca-certificates

# Set the working directory
WORKDIR /app

# Copy only the binary from the builder stage
COPY --from=builder /out/scheduler-extender .

# Expose the application's port
EXPOSE 8080

# Run the application
CMD ["./scheduler-extender"]

# docker build -f k8s/scheduler/extender/Dockerfile -t docker.io/devandrejesus/snapshot-locality-extender:latest .