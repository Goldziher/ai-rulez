#!/usr/bin/env bash
printf '%s\n' "PS1='\$(git branch --show-current 2>/dev/null) %# '"
