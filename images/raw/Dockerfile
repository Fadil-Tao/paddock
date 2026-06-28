FROM debian:bookworm

ENV DEBIAN_FRONTEND=noninteractive
ENV DISPLAY=:99
ENV VNC_PORT=5900
ENV NOVNC_PORT=6080
ENV TTYD_PORT=7681
ENV CDP_PORT=9222

# VNC + display + browser + terminal deps
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    git \
    wget \
    ca-certificates \
    build-essential \
    tini \
    python3 \
    python3-pip \
    xvfb \
    x11vnc \
    novnc \
    websockify \
    openbox \
    bash \
    chromium \
    fonts-liberation \
    fonts-noto-color-emoji \
    libnss3 \
    libatk-bridge2.0-0 \
    libdrm2 \
    libxcomposite1 \
    libxdamage1 \
    libxrandr2 \
    libgbm1 \
    libasound2 \
    libxss1 \
    procps \
    sudo \
    && rm -rf /var/lib/apt/lists/*

# ttyd — multi-arch aware
RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "x86_64" ]; then TTYD_ARCH="x86_64"; \
    elif [ "$ARCH" = "aarch64" ]; then TTYD_ARCH="aarch64"; \
    else echo "Unsupported arch: $ARCH" && exit 1; fi && \
    curl -fsSL "https://github.com/tsl0922/ttyd/releases/download/1.7.7/ttyd.${TTYD_ARCH}" \
    -o /usr/local/bin/ttyd && \
    chmod +x /usr/local/bin/ttyd

# Node.js 24 via NodeSource (global, all users)
RUN curl -fsSL https://deb.nodesource.com/setup_24.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# OpenCode
RUN npm install -g opencode-ai

# Non-root user
RUN useradd -m -s /bin/bash sandbox && \
    echo "sandbox ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

# Pre-create X11 socket dir with correct permissions (fixes non-root Xvfb)
RUN mkdir -p /tmp/.X11-unix && chmod 1777 /tmp/.X11-unix

COPY scripts/start.sh /start.sh
RUN chmod +x /start.sh

EXPOSE 6080 7681 9222

WORKDIR /home/sandbox
USER sandbox

ENTRYPOINT ["tini", "--"]
CMD ["/start.sh"]

