#!/bin/sh
set -eu

: "${SENTRY_FRONTEND_DSN:=}"
: "${SENTRY_ENVIRONMENT:=production}"
: "${SENTRY_RELEASE:=}"
: "${SENTRY_FRONTEND_TRACES_SAMPLE_RATE:=0}"

envsubst '${SENTRY_FRONTEND_DSN} ${SENTRY_ENVIRONMENT} ${SENTRY_RELEASE} ${SENTRY_FRONTEND_TRACES_SAMPLE_RATE}' \
  < /usr/share/nginx/html/env.template.js \
  > /usr/share/nginx/html/env.js
