FROM docker.io/library/ubuntu:24.04

# Install base development tools
RUN apt-get update && apt-get install -y \
    curl \
    git \
    make \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js (demonstrates what a feature's install.sh would do)
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && \
    apt-get install -y nodejs && \
    rm -rf /var/lib/apt/lists/*
