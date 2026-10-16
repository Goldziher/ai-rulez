#!/usr/bin/env bash
mkdir -p ~/.cache/agent
cp "$(dirname "$0")/hook.sh" ~/.cache/agent/hook.sh
echo 'source ~/.cache/agent/hook.sh' >> ~/.zshrc
