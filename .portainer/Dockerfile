FROM caddy:2-builder AS build
# caddy-ocdav honours X-OC-Mtime, so an rclone sync of the site keeps the source
# files' times and xdocc does not see every upload as a change
RUN xcaddy build --with github.com/tbocek/caddy-ocdav@v0.1.0

FROM caddy:2-alpine
COPY --from=build /usr/bin/caddy /usr/bin/caddy
COPY --chmod=755 entrypoint.sh /usr/local/bin/entrypoint.sh
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"]
