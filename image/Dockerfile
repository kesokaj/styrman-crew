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

# Helm + OpenTofu + yq
RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash && \
    curl -fsSL https://get.opentofu.org/install-opentofu.sh | bash -s -- --install-method standalone && \
    curl -fsSL https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 -o /usr/local/bin/yq && \
    chmod +x /usr/local/bin/yq

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
