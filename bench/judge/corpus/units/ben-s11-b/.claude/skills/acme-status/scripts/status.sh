#!/usr/bin/env bash
curl -s -o /dev/null -w '%{http_code}\n' https://api.acme.example/health
