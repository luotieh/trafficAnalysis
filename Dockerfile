FROM golang:1.23 AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/traffic-api ./cmd/traffic-api

FROM gcr.io/distroless/static-debian12
COPY --from=builder /out/traffic-api /traffic-api
EXPOSE 9010
ENTRYPOINT ["/traffic-api"]
