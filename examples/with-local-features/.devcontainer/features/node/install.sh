#!/bin/bash
set -e

echo "Installing Node.js..."

apt-get update
apt-get install -y curl

curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
apt-get install -y nodejs

node --version
npm --version

rm -rf /var/lib/apt/lists/*
