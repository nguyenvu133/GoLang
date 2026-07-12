FROM golang:1.22-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app .

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /out/app /app/app
EXPOSE 9000
CMD ["/app/app"]
