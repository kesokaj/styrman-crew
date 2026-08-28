# Stage 1: Build custom Caddy with all plugins + our ingress module
FROM caddy:2-builder AS builder

COPY ingress/ /src/caddy-ingress/

RUN xcaddy build \
    --with github.com/greenpau/caddy-security \
    --with github.com/caddy-dns/cloudflare \
    --with github.com/mholt/caddy-ratelimit \
    --with github.com/mholt/caddy-l4 \
    --with github.com/spgo/devbox/caddy/ingress=/src/caddy-ingress

# Stage 2: Runtime
FROM caddy:2

COPY --from=builder /usr/bin/caddy /usr/bin/caddy

# Version label for image introspection
ARG DEVBOX_VERSION=0.1.0
LABEL devbox.version=${DEVBOX_VERSION}

# Verify the build includes our modules
RUN caddy list-modules | grep -E "(security|cloudflare|rate_limit|layer4|ingress)"

EXPOSE 80 443 2019
