# Use the official Golang image to build the application
FROM golang:1-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN go build -o sb8200-exporter

# Use a minimal image for the final container
FROM alpine:latest

# Set the working directory
WORKDIR /root/

# Copy the compiled binary from the builder stage
COPY --from=builder /app/sb8200-exporter .

# Expose the port the exporter listens on
EXPOSE 9143

# Set environment variables (modify as needed)
ENV SB8200_HOST=192.168.100.1
ENV SB8200_USER=admin
ENV SB8200_PASS=password

# Command to run the application
CMD ["./sb8200-exporter"]
