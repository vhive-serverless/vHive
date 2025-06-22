FROM golang:1.22-alpine AS builder

WORKDIR /workspace

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the scheduler
RUN CGO_ENABLED=0 GOPROXY=direct GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o snapshot-aware-scheduler ./k8s/scheduler

# Final stage
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/snapshot-aware-scheduler .
USER 65532:65532
ENTRYPOINT ["/snapshot-aware-scheduler"]