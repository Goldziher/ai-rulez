#!/usr/bin/env bash
aws s3 sync ./dist "s3://$1" --profile "$AWS_PROFILE"
