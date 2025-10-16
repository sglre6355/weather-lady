FROM golang AS builder
WORKDIR /app
# Install buf
RUN curl -sSL https://github.com/bufbuild/buf/releases/latest/download/buf-Linux-x86_64 -o /usr/local/bin/buf && chmod +x /usr/local/bin/buf
# Copy buf configuration files
COPY buf.yaml buf.gen.yaml ./
# Copy proto files
COPY api/ ./api/
# Copy Go module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download
# Copy remaining source files
COPY . .
# Build the application
RUN go build -o weather-lady .

FROM gcr.io/distroless/cc
COPY --from=builder /app/weather-lady /
CMD ["/weather-lady"]
