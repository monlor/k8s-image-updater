FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o k8s-image-updater

FROM alpine:3.19

ENV GIN_MODE=release

WORKDIR /app
COPY --from=builder /app/k8s-image-updater .

EXPOSE 8080
CMD ["./k8s-image-updater"] 