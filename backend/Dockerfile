FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
RUN go install github.com/swaggo/swag/cmd/swag@latest
COPY . .
RUN swag init -g main.go -o docs
RUN CGO_ENABLED=0 GOOS=linux go build -o /rpanel main.go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /rpanel ./rpanel
COPY --from=builder /app/storage ./storage
EXPOSE 8500
CMD ["./rpanel"]
