# styrman-runtime — the per-project microVM image (AMD64 only, R11/ADR-001).
# Pure tooling: no server binary inside (ADR-002); the styrman server drives
# everything via docker exec.
FROM debian:trixie

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
# Every remaining tool is pinned by version AND verified by sha256 (audit
# 2026-09-02). OpenTofu was a `curl | bash` installer script — an
# unauthenticated remote shell script running as root at build time, fetching
# whatever version was current that day — and yq used
# `releases/latest/download`. Every checksum
# below was cross-checked against the publisher's own published sums, not just
# computed from whatever the download returned. Bump these deliberately and
# together (specs/styrman/runtime-image.md).
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

# Go (latest stable) + Node 22
RUN GO_VERSION=$(curl -fsSL 'https://go.dev/dl/?mode=json' | jq -r '.[0].version') && \
    curl -fsSL "https://go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz && \
    ln -s /usr/local/go/bin/go /usr/local/bin/go && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && \
    apt-get install --no-install-recommends -y nodejs && rm -rf /var/lib/apt/lists/*

# The agents: Claude Code (npm) and Antigravity CLI (single Go binary).
RUN npm install -g @anthropic-ai/claude-code && \
    curl -fsSL https://antigravity.google/cli/install.sh | bash && \
    { mv /root/.local/bin/agy /usr/local/bin/agy 2>/dev/null || true; } && \
    claude --version && agy --version

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
# Agent harness (claude + agy), vendored snapshot of devshell_prompt/harness
# (R5/ADR-030): the base image carries identical defaults for every project;
# the entrypoint seeds it into each home write-once. See harness/VENDORED.md.
COPY harness/ /styrman/harness/
ARG STYRMAN_VERSION=0.0.0
RUN echo "${STYRMAN_VERSION}" > /styrman/VERSION && chmod +x /styrman/entrypoint.sh && \
    install -m 755 /styrman/configs/styrman-setup /usr/local/bin/styrman-setup && \
    install -m 755 /styrman/configs/styrman-git-credential /usr/local/bin/styrman-git-credential

# Credential broker git helper (ADR-030): a SYSTEM gitconfig makes every
# project use the broker for HTTPS credentials — no per-home setup, no stored
# token. useHttpPath=false so one credential covers a whole host.
RUN git config --system credential.helper /usr/local/bin/styrman-git-credential && \
    git config --system credential.useHttpPath false

ENTRYPOINT ["/usr/bin/tini", "-s", "--", "/styrman/entrypoint.sh"]
