#!/bin/sh
set -eu

DATA_DIR="${STORAGE_ROOT:-/data}"

mkdir -p "$DATA_DIR/.trash" "$DATA_DIR/.thumbnails" "$DATA_DIR/.chunks"

chown -R explorer:explorer "$DATA_DIR"
chown -R explorer:explorer /app

# ---------------------------------------------------------------------------
# Generate .env / JWT_SECRET if needed
# ---------------------------------------------------------------------------
ENV_FILE="/app/.env"
ENV_EXAMPLE_FILE="/app/.env.example"

if [ ! -f "$ENV_FILE" ] && [ -f "$ENV_EXAMPLE_FILE" ]; then
  cp "$ENV_EXAMPLE_FILE" "$ENV_FILE"
  chown explorer:explorer "$ENV_FILE"
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

# Drop privileges and run the application as the explorer user.
exec su-exec explorer /usr/local/bin/go-file-explorer
