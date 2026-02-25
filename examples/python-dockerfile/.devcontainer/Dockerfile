FROM docker.io/library/python:3.12-bookworm

# Install development tools
RUN apt-get update && apt-get install -y \
    git \
    make \
    curl \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /workspaces/python-dockerfile

# Install Python development dependencies
RUN pip install --no-cache-dir \
    pytest \
    pytest-cov \
    black \
    flake8
