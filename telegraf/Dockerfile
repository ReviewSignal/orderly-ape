FROM golang:1.23-alpine AS builder
RUN apk add --no-cache --virtual .build-deps make git wget
WORKDIR /go/telegraf
RUN git clone --branch v1.32.0 --depth 1 https://github.com/influxdata/telegraf.git ./
RUN wget -O- https://patch-diff.githubusercontent.com/raw/influxdata/telegraf/pull/15853.patch | git apply
RUN go build -tags "custom,inputs.statsd,outputs.influxdb_v2" -o telegraf ./cmd/telegraf

FROM alpine:3.20

RUN echo 'hosts: files dns' >> /etc/nsswitch.conf
RUN apk add --no-cache iputils ca-certificates net-snmp-tools procps lm_sensors tzdata su-exec libcap && \
    update-ca-certificates

ENV TELEGRAF_VERSION=1.32.0

RUN ARCH= && \
    case "$(apk --print-arch)" in \
    x86_64) ARCH='amd64';; \
    *) echo "Unsupported architecture: $(apk --print-arch)"; exit 1;; \
    esac && \
    set -ex && \
    mkdir -p /etc/telegraf && \
    addgroup -S telegraf && \
    adduser -S telegraf -G telegraf && \
    chown -R telegraf:telegraf /etc/telegraf
COPY --from=builder /go/telegraf/telegraf /usr/bin/

EXPOSE 8125/udp 8092/udp 8094

COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
CMD ["telegraf"]
