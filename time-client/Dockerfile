FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy source code
COPY . .

# Build the application
RUN cd time-client && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o time-client .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/time-client/time-client .

EXPOSE 8080

CMD ["./time-client"] 