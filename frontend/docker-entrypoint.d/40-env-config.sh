#!/bin/sh
set -eu

: "${SENTRY_FRONTEND_DSN:=}"
: "${SENTRY_ENVIRONMENT:=production}"
: "${SENTRY_RELEASE:=}"
: "${SENTRY_FRONTEND_TRACES_SAMPLE_RATE:=0}"

export SENTRY_FRONTEND_DSN
export SENTRY_ENVIRONMENT
export SENTRY_RELEASE
export SENTRY_FRONTEND_TRACES_SAMPLE_RATE

envsubst '${SENTRY_FRONTEND_DSN} ${SENTRY_ENVIRONMENT} ${SENTRY_RELEASE} ${SENTRY_FRONTEND_TRACES_SAMPLE_RATE}' \
  < /usr/share/nginx/html/env.template.js \
  > /usr/share/nginx/html/env.js
