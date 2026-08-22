FROM node:22.23.1-alpine AS node

# ---------- Build stage ----------
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache make git libstdc++ libgcc

COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -sf ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -sf ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

WORKDIR /src

COPY Makefile go.mod go.sum ./
RUN go mod download

COPY frontend/package.json frontend/package-lock.json frontend/
RUN make frontend-deps

COPY . .
RUN make build

# ---------- Runtime stage ----------
FROM alpine:3.20 AS runtime

ARG DATA_DIR=/data
ARG CONFIG_DIR=/config
ARG UID=1000
ARG GID=1000

RUN apk add --no-cache ca-certificates tini

RUN addgroup -g ${GID} dockvmap && \
    adduser -D -H -u ${UID} -G dockvmap -h ${DATA_DIR} dockvmap

COPY --from=builder /src/bin/dockvmap /usr/local/bin/dockvmap
COPY config.sample.yaml /etc/dockvmap/config.sample.yaml
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

RUN mkdir -p ${DATA_DIR} ${CONFIG_DIR} && chown -R dockvmap:dockvmap ${DATA_DIR} ${CONFIG_DIR}

USER dockvmap

WORKDIR ${DATA_DIR}

VOLUME ["${DATA_DIR}", "${CONFIG_DIR}"]

EXPOSE 8080 5000

ENTRYPOINT ["/sbin/tini", "--", "/entrypoint.sh"]

CMD []
