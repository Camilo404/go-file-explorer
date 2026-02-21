#!/bin/sh
set -eu

ENV_FILE="/app/.env"
ENV_EXAMPLE_FILE="/app/.env.example"

if [ ! -f "$ENV_FILE" ] && [ -f "$ENV_EXAMPLE_FILE" ]; then
  cp "$ENV_EXAMPLE_FILE" "$ENV_FILE"
fi

needs_jwt=false
if [ -z "${JWT_SECRET:-}" ] || [ "${JWT_SECRET:-}" = "change-me-in-production" ]; then
  needs_jwt=true
fi

if [ "$needs_jwt" = true ]; then
  generated_jwt_secret="$(head -c 48 /dev/urandom | base64 | tr -d '\n')"
  export JWT_SECRET="$generated_jwt_secret"

  if [ -f "$ENV_FILE" ]; then
    if grep -q '^JWT_SECRET=' "$ENV_FILE"; then
      sed -i "s|^JWT_SECRET=.*|JWT_SECRET=${generated_jwt_secret}|" "$ENV_FILE"
    else
      printf '\nJWT_SECRET=%s\n' "$generated_jwt_secret" >> "$ENV_FILE"
    fi
  fi
fi

exec /usr/local/bin/go-file-explorer
