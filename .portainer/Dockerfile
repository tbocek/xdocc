FROM caddy:2-builder AS build
RUN xcaddy build --with github.com/mholt/caddy-webdav

FROM caddy:2-alpine
COPY --from=build /usr/bin/caddy /usr/bin/caddy
