FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    fd-find \
    fzf \
    git \
    jq \
    less \
    neovim \
    openssh-client \
    ripgrep \
    sudo \
    zsh \
 && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
      -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
 && chmod 644 /usr/share/keyrings/githubcli-archive-keyring.gpg \
 && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
      > /etc/apt/sources.list.d/github-cli.list \
 && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
 && apt-get update \
 && apt-get install -y --no-install-recommends gh nodejs \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*

ARG GO_VERSION=1.25.3
RUN ARCH="$(dpkg --print-architecture)" \
 && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" \
      | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/home/dev/go/bin:${PATH}" \
    GOPATH="/home/dev/go"

RUN GOBIN=/usr/local/bin /usr/local/go/bin/go install github.com/bokwoon95/wgo@latest

RUN useradd -m -s /bin/bash dev \
 && chown -R dev:dev /home/dev \
 && echo "dev ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/dev \
 && chmod 0440 /etc/sudoers.d/dev

ARG CLAUDE_CODE_VERSION=latest
ARG PI_CODING_AGENT_VERSION=latest
RUN npm install -g \
      @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION} \
      @mariozechner/pi-coding-agent@${PI_CODING_AGENT_VERSION}

COPY github_known_hosts /etc/ssh/ssh_known_hosts
RUN chmod 644 /etc/ssh/ssh_known_hosts

ENV DEVCONTAINER=true

RUN mkdir -p /workspace && chown -R dev:dev /workspace
WORKDIR /workspace
USER dev
