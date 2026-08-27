FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
# -s -w strip the debug info, -trimpath keeps build paths out of the binary
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
	go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
	-o /out/xdocc ./cmd/xdocc

FROM alpine:3.24

ARG VERSION=dev
LABEL org.opencontainers.image.title="xdocc" \
	org.opencontainers.image.description="Static site generator; filenames are the configuration" \
	org.opencontainers.image.source="https://github.com/tbocek/xdocc" \
	org.opencontainers.image.licenses="MIT" \
	org.opencontainers.image.version="${VERSION}"

# tzdata because a date in front matter is formatted in the local zone, and
# a static binary has no zone database of its own. Nothing else is needed:
# xdocc reads files, writes files, and never opens a socket.
RUN apk add --no-cache tzdata \
	&& addgroup -g 1000 xdocc \
	&& adduser -u 1000 -G xdocc -h /home/xdocc -s /bin/sh -D xdocc \
	&& install -d -o 1000 -g 1000 /srv/site /srv/www /var/cache/xdocc

COPY --from=build /out/xdocc /usr/local/bin/xdocc

USER xdocc
WORKDIR /srv

ENTRYPOINT ["/usr/local/bin/xdocc"]
CMD ["-s", "/srv/site", "-o", "/srv/www", "-c", "/var/cache/xdocc/cache.gob", "-w"]
