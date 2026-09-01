# Stage 1: Build custom Caddy with all plugins + our ingress module.
# Everything is PINNED (operator rule: no mutable tags — :latest, :2, floating
# plugins). A floating builder made the tag→binary mapping non-deterministic:
# the binary depended on the day it was built, so a rollback could not restore
# what actually ran. These pins are what the 2026-09-01 smoke-tested build
# resolved; bump them together and re-run that smoke test (OAuth login) when
# raising the Caddy version.
FROM caddy:2.11.4-builder AS builder

COPY ingress/ /src/caddy-ingress/

RUN xcaddy build \
    --with github.com/greenpau/caddy-security@v1.1.64 \
    --with github.com/caddy-dns/cloudflare@v0.2.4 \
    --with github.com/mholt/caddy-ratelimit@v0.1.0 \
    --with github.com/spgo/devbox/caddy/ingress=/src/caddy-ingress

# Stage 2: Runtime
FROM caddy:2.11.4

COPY --from=builder /usr/bin/caddy /usr/bin/caddy

# Version label for image introspection
ARG DEVBOX_VERSION=0.1.0
LABEL devbox.version=${DEVBOX_VERSION}

# Verify the build includes our modules
RUN caddy list-modules | grep -E "(security|cloudflare|rate_limit|ingress)"

EXPOSE 80 443
