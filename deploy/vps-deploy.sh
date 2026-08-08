#!/usr/bin/env bash
#
# Deploys the M-80 production stack from images already published to GHCR.
#
# The caller checks the repository out at the commit being deployed *before*
# running this. The script deliberately does no git work itself: bash reads a
# script incrementally, so a script that rewrote its own file mid-run would be
# a fine way to lose an afternoon.
#
# Usage:
#   deploy/vps-deploy.sh <image-tag>
#
# Variables:
#   PROFILE         Compose profile (default: prod)
#   HEALTH_URL      URL polled until the API answers. Defaults to
#                   https://$API_DOMAIN/health, and API_DOMAIN falls back to
#                   API_DOMAIN_PROD in backend/.env
#   HEALTH_TIMEOUT  seconds to wait before giving up and rolling back (default: 90)
#   STATE_FILE      where the deployed tags are remembered (default: deploy/.deploy-state)
#   SKIP_BACKUP     set to 1 to skip the pre-deploy dump - rehearsals only

set -Eeuo pipefail

TAG="${1:-}"

PROFILE="${PROFILE:-prod}"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATE_FILE="${STATE_FILE:-$REPO_DIR/deploy/.deploy-state}"
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-90}"

log() { printf '%s  %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"; }
die() { printf '%s  ERROR: %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >&2; exit 1; }

[ -n "$TAG" ] || die "usage: deploy/vps-deploy.sh <image-tag>"

compose() { IMAGE_TAG="$1" docker compose --profile "$PROFILE" "${@:2}"; }

resolve_health_url() {
  if [ -n "${HEALTH_URL:-}" ]; then
    return
  fi

  local domain="${API_DOMAIN:-}"
  if [ -z "$domain" ] && [ -f "$REPO_DIR/backend/.env" ]; then
    domain="$(sed -n 's/^API_DOMAIN_PROD=//p' "$REPO_DIR/backend/.env" | tr -d '"' | head -1)"
  fi

  [ -n "$domain" ] || die "cannot work out the health URL: set HEALTH_URL or API_DOMAIN"
  HEALTH_URL="https://${domain}/health"
}

# Polls until the API answers or the budget runs out. A deploy that comes up
# broken has to be *detected*, not assumed - the rollback below is worthless
# otherwise.
wait_for_health() {
  local deadline=$(( $(date +%s) + HEALTH_TIMEOUT ))

  while [ "$(date +%s)" -lt "$deadline" ]; do
    if curl --fail --silent --show-error --max-time 5 "$HEALTH_URL" >/dev/null 2>&1; then
      return 0
    fi
    sleep 3
  done

  return 1
}

deploy_tag() {
  local tag="$1"
  log "pulling images tagged ${tag}..."
  # A failed pull is a dead end rather than a rollback: nothing was replaced,
  # so there is nothing to undo.
  compose "$tag" pull || die "could not pull the images for ${tag} - nothing was changed"
  log "starting the stack..."
  compose "$tag" up -d || die "compose up failed for ${tag}"
}

cd "$REPO_DIR/backend"
resolve_health_url

previous_tag=""
if [ -f "$STATE_FILE" ]; then
  # shellcheck disable=SC1090
  source "$STATE_FILE"
  previous_tag="${CURRENT_TAG:-}"
fi

# Before anything else, because the API runs its migrations on boot and a
# migration is the one step of a deploy that a rollback cannot undo.
#
# The one case where there is nothing to dump is the very first deploy: db-prod
# has never started, and db-backup.sh would exit non-zero on `compose exec`
# against a service that is not running - taking the whole deploy down with it,
# before a single image had been pulled. So the backup is skipped when, and only
# when, no database container is running.
if [ "${SKIP_BACKUP:-0}" = "1" ]; then
  log "skipping the pre-deploy dump (SKIP_BACKUP=1)"
elif [ -z "$(compose "$TAG" ps --status running -q "db-${PROFILE}" 2>/dev/null)" ]; then
  log "no database container running - skipping the pre-deploy dump (first deploy)"
else
  log "taking a pre-deploy dump..."
  PROFILE="$PROFILE" "$REPO_DIR/backend/scripts/db-backup.sh"
fi

deploy_tag "$TAG"

log "waiting for ${HEALTH_URL} (up to ${HEALTH_TIMEOUT}s)..."
if wait_for_health; then
  printf 'CURRENT_TAG=%s\nPREVIOUS_TAG=%s\n' "$TAG" "$previous_tag" > "$STATE_FILE"
  log "ok: ${TAG} is live"
  exit 0
fi

log "the API never became healthy on ${TAG}"

if [ -z "$previous_tag" ] || [ "$previous_tag" = "$TAG" ]; then
  die "no earlier tag to fall back to - the stack is left as is, for inspection"
fi

# Images only. If this deploy also changed docker-compose.yml or the Caddyfile,
# check the previous commit out and run this again - the working tree is not
# rewound here, on purpose (see the header).
log "rolling back to ${previous_tag}..."
deploy_tag "$previous_tag"

if wait_for_health; then
  log "rolled back to ${previous_tag}"
else
  log "the rollback did not come up healthy either - this one needs hands"
fi

die "deploy of ${TAG} failed"
