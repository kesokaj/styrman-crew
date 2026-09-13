# styrman-runtime — the per-project microVM image (AMD64 only, R11/ADR-001).
# Pure tooling: no server binary inside (ADR-002); the styrman server drives
# everything via docker exec.
# sh-agent, the harness installer, is built from the vendored styrman-harness
# mirror (harness/, see harness/VENDORED.md) in a builder stage pinned to the
# same Go toolchain as deploy/Dockerfile.server. Static binary; the runtime
# stage only copies it (ADR-062).
FROM golang:1.25.13-trixie@sha256:6be3daa12135c38c177e2fbbf38ef2ad000d09b34bfa9f73dd7ca785bff134a5 AS harness-builder
WORKDIR /src
COPY harness/ /src/
# GOPROXY=off: the module has no requirements; make that a build failure,
# not a network fetch, if it ever changes.
RUN CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off go build -trimpath -ldflags='-s -w' -o /out/sh-agent ./cmd/sh-agent

FROM debian:trixie@sha256:6788062a1b42ac281f053ac876170b79a3eaed5d61383b8ed7eaca6c6965f3b1

ENV DEBIAN_FRONTEND=noninteractive
ENV TERM=xterm-256color

# Base tools + everything the entrypoint needs (e2fsprogs for the per-project
# ext4 docker image, tini as the PID-1 reaper). The keyring packages
# (dbus/gnome-keyring/libsecret-tools) were dropped with the broker keyring tier
# (ADR-040) — every credential is env/file/git-helper now.
RUN apt-get update && apt-get install --no-install-recommends -y \
    ca-certificates curl wget gnupg lsb-release locales sudo tini procps psmisc \
    git jq ripgrep fd-find bat tmux htop btop less vim nano unzip zip tree \
    openssh-client rsync make build-essential python3 python3-venv python3-pip \
    python-is-python3 e2fsprogs \
    iproute2 iputils-ping dnsutils netcat-openbsd \
    bash-completion lsof mtr-tiny traceroute net-tools gettext-base openssl \
    shellcheck \
    && rm -rf /var/lib/apt/lists/* \
    && sed -i 's/# en_US.UTF-8/en_US.UTF-8/' /etc/locale.gen && locale-gen

ENV LANG=en_US.UTF-8 LANGUAGE=en_US:en LC_ALL=en_US.UTF-8

# Docker (for per-project DinD, ADR-004), gh, gcloud, kubectl — official APT repos.
RUN install -m 0755 -d /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc && \
    echo "deb [signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian trixie stable" > /etc/apt/sources.list.d/docker.list && \
    curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg -o /etc/apt/keyrings/githubcli.gpg && \
    echo "deb [signed-by=/etc/apt/keyrings/githubcli.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list && \
    curl -fsSL https://packages.cloud.google.com/apt/doc/apt-key.gpg -o /etc/apt/keyrings/cloud.google.asc && \
    echo "deb [signed-by=/etc/apt/keyrings/cloud.google.asc] https://packages.cloud.google.com/apt cloud-sdk main" > /etc/apt/sources.list.d/google-cloud-sdk.list && \
    apt-get update && apt-get install --no-install-recommends -y \
    docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin \
    gh google-cloud-cli google-cloud-cli-gke-gcloud-auth-plugin kubectl \
    && rm -rf /var/lib/apt/lists/*

# hadolint is PINNED with a checksum, unlike the `latest` fetches around it:
# an unpinned tool makes the tag->image mapping non-deterministic, so the
# auto-updater's rollback would not restore the image the old tag produced
# (security review, 2026-09-01).
#
# What is pinned, and how (audit 2026-09-02, completed 2026-09-13 — ADR-069):
# OpenTofu, yq, hadolint, the Go toolchain and Node are fetched by VERSION and
# verified against the publisher's own published sha256 (never a sum computed
# from whatever the download returned); Claude Code is pinned by npm version
# (its postinstall is what installs the CLI, so no --ignore-scripts);
# chrome-devtools-mcp is pinned further down. The ONE thing that floats is
# the Antigravity CLI (`agy`): Google publishes it as latest-only behind a
# manifest whose sha512 the installer verifies, offers no versioned artifact,
# and the CLI self-updates at run time anyway — a build-time pin would be
# theatre, so the floating install is an accepted, recorded exception.
# Before 2026-09-02, OpenTofu and Helm were `curl | bash` too; Helm was
# removed, OpenTofu pinned. Bump the ARG pairs deliberately and together
# (specs/styrman/runtime-image.md names the procedure).
ARG TOFU_VERSION=1.12.6
ARG TOFU_SHA256=5dc43da4f750f33873dc25e94587128709e819e544b7be9016b255316153c3a8
ARG YQ_VERSION=4.53.6
ARG YQ_SHA256=c5f056448f973ae7d39b5401949648a78f2dc1947d6a8eb65be60d5c504b9385
ARG HADOLINT_VERSION=2.15.1
ARG HADOLINT_SHA256=c7187db94eeeeca956519a6af171adc31453941a1e777961f6e680f697c8c507
# OpenTofu + yq + hadolint
# hadolint is the Dockerfile checker behind the editor's lint gutter
# (internal/httpapi/check.go); shellcheck, its shell counterpart, comes from
# apt above. Helm was REMOVED (operator directive 2026-09-02) — it was the
# worst of the unpinned fetches (`curl … get-helm-3 | bash`) and is not wanted
# in the image; a project that needs it can install it itself.
RUN set -eux; \
    curl -fsSL -o /tmp/tofu.zip "https://github.com/opentofu/opentofu/releases/download/v${TOFU_VERSION}/tofu_${TOFU_VERSION}_linux_amd64.zip"; \
    echo "${TOFU_SHA256}  /tmp/tofu.zip" | sha256sum -c -; \
    unzip -q -o -d /tmp/tofu /tmp/tofu.zip; \
    install -m 0755 /tmp/tofu/tofu /usr/local/bin/tofu; \
    curl -fsSL -o /usr/local/bin/yq "https://github.com/mikefarah/yq/releases/download/v${YQ_VERSION}/yq_linux_amd64"; \
    echo "${YQ_SHA256}  /usr/local/bin/yq" | sha256sum -c -; \
    chmod 0755 /usr/local/bin/yq; \
    curl -fsSL -o /usr/local/bin/hadolint "https://github.com/hadolint/hadolint/releases/download/v${HADOLINT_VERSION}/hadolint-Linux-x86_64"; \
    echo "${HADOLINT_SHA256}  /usr/local/bin/hadolint" | sha256sum -c -; \
    chmod 0755 /usr/local/bin/hadolint; \
    rm -rf /tmp/tofu.zip /tmp/tofu

# Go + Node, pinned (sums from go.dev/dl and nodejs.org/dist/vX/SHASUMS256.txt,
# read 2026-09-13; these are the versions production already ran, now
# explicit). A changed upstream tarball fails the build loudly.
ARG GO_VERSION=1.27.1
ARG GO_SHA256=63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445
ARG NODE_VERSION=22.23.2
ARG NODE_SHA256=d60acfe00a2932254bb0ad20e01b0d74397a0875595de719654b214f4b03f307
RUN set -eux; \
    curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"; \
    echo "${GO_SHA256}  /tmp/go.tgz" | sha256sum -c -; \
    tar -C /usr/local -xzf /tmp/go.tgz; rm -f /tmp/go.tgz; \
    ln -s /usr/local/go/bin/go /usr/local/bin/go && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt; \
    curl -fsSL -o /tmp/node.txz "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz"; \
    echo "${NODE_SHA256}  /tmp/node.txz" | sha256sum -c -; \
    tar -C /usr/local --strip-components=1 -xJf /tmp/node.txz --exclude='*/CHANGELOG.md' --exclude='*/README.md' --exclude='*/LICENSE'; \
    rm -f /tmp/node.txz; node --version; npm --version

# The agents: Claude Code (npm, pinned; its postinstall installs the CLI) and
# the Antigravity CLI (floats by upstream design — see the note above; the
# installer downloads to a file first so a failed fetch cannot run a partial
# script).
ARG CLAUDE_CODE_VERSION=2.1.270
RUN set -eux; \
    npm install -g "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}"; \
    curl -fsSL -o /tmp/agy-install.sh https://antigravity.google/cli/install.sh; \
    bash /tmp/agy-install.sh; rm -f /tmp/agy-install.sh; \
    { mv /root/.local/bin/agy /usr/local/bin/agy 2>/dev/null || true; }; \
    claude --version && agy --version

# Browser view (ADR-068): one headless Chromium per project, driven by the
# operator through the SPA's screencast and by the agents through
# chrome-devtools-mcp (pinned by exact npm version; it attaches to the RUNNING
# Chromium over CDP and downloads no browser of its own). Debian's own
# chromium package: its version moves with the archive's security updates,
# exactly like docker-ce above, so it is pinned by the archive, not by a
# sha256. fonts-liberation gives pages metric-compatible Arial/Times/Courier
# instead of boxes. socat publishes DevTools on the guest's bridge address:
# new-headless Chromium ignores --remote-debugging-address and binds loopback
# only (verified 2026-09-11), and the styrman server reaches the guest over the
# bridge, never through loopback. Sits BEFORE the base-package snapshot below so apt
# persistence (ADR-028) never records chromium as a user install. Launcher:
# configs/styrman-browser → /styrman/bin (installed with the others below).
# --ignore-scripts: no transitive package runs a lifecycle script as root at
# build time (security review 2026-09-11; the package needs none — proven by
# the --help run). The resolved GLOBAL tree (claude-code included) is
# recorded at /styrman/npm-global.tree.json so two builds of one version can
# be diffed: only the top-level versions are pinned, the rest resolves by
# semver.
ARG CHROME_DEVTOOLS_MCP_VERSION=1.9.0
RUN mkdir -p /styrman && apt-get update && apt-get install --no-install-recommends -y chromium fonts-liberation socat \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g --ignore-scripts "chrome-devtools-mcp@${CHROME_DEVTOOLS_MCP_VERSION}" \
    && npm ls -g --all --json > /styrman/npm-global.tree.json \
    && chromium --version && chrome-devtools-mcp --help >/dev/null

# In-guest defaults: DNS template (Kata has no embedded Docker DNS) and a
# package snapshot for drift debugging.
RUN printf "nameserver 8.8.8.8\nnameserver 1.1.1.1\n" > /etc/resolv.conf.styrman && \
    dpkg-query -W -f='${Package}\n' | sort > /etc/styrman-base-packages

COPY entrypoint.sh /styrman/entrypoint.sh
COPY configs/ /styrman/configs/
# Default shell dotfiles (ADR-031): the base image carries them (the single
# source of truth since internal/dotfiles was removed); the entrypoint seeds
# each per-project home write-once.
COPY skel/ /styrman/skel/
# Agent harness (claude + agy): a mirror of styrman-harness at harness/COMMIT
# (R5/ADR-030, ADR-062) plus the sh-agent binary built from it above. The base
# image carries identical defaults for every project; the entrypoint seeds
# them into each home write-once with `sh-agent install`. See harness/VENDORED.md.
COPY harness/ /styrman/harness/
COPY --from=harness-builder /out/sh-agent /styrman/bin/sh-agent
ARG STYRMAN_VERSION=0.0.0
RUN echo "${STYRMAN_VERSION}" > /styrman/VERSION && chmod +x /styrman/entrypoint.sh && \
    chmod -R go-w /styrman/harness && \
    install -m 755 /styrman/configs/styrman-setup /usr/local/bin/styrman-setup && \
    install -m 755 /styrman/configs/styrman-git-credential /usr/local/bin/styrman-git-credential && \
    install -m 755 /styrman/configs/styrman-browser /styrman/bin/styrman-browser

# Credential broker git helper (ADR-030): a SYSTEM gitconfig makes every
# project use the broker for HTTPS credentials — no per-home setup, no stored
# token. useHttpPath=false so one credential covers a whole host.
RUN git config --system credential.helper /usr/local/bin/styrman-git-credential && \
    git config --system credential.useHttpPath false

ENTRYPOINT ["/usr/bin/tini", "-s", "--", "/styrman/entrypoint.sh"]
