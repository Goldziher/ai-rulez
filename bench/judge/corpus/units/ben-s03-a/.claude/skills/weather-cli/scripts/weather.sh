#!/usr/bin/env bash
curl -s "https://api.weather.example/v1/current?city=$(printf %s "$1" | jq -sRr @uri)"
