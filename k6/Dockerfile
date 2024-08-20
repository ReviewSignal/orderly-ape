FROM ghcr.io/grafana/xk6 AS builder

RUN xk6 build \
    --with github.com/LeonAdato/xk6-output-statsd

FROM alpine:3.18 AS release

RUN adduser -D -u 12345 -g 12345 k6
COPY --from=builder /xk6/k6 /usr/bin/k6

USER k6
WORKDIR /home/k6

ENTRYPOINT ["k6"]
