FROM public.ecr.aws/docker/library/golang:1.25 AS builder

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

WORKDIR /src

COPY . .

RUN go build -mod=vendor -o /out/traffic-api ./cmd/traffic-api
RUN go build -mod=vendor -o /out/traffic-admin ./cmd/traffic-admin

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/traffic-api /traffic-api
COPY --from=builder /out/traffic-admin /traffic-admin

EXPOSE 9010
ENTRYPOINT ["/traffic-api"]
