.PHONY: init dev front back-up back-down back-logs clean db-backup db-backup-list db-restore db-anonymize ext-dev ext-build

# LOCAL DEVELOPMENT (DOCKER-FIRST)

# The carrier clients (backend/carriers) and the extension are private submodules.
# The backend does not compile without them, so run this once after cloning.
init:
	git submodule update --init --recursive

# Master Command: Backend setup and run native frontend
dev: back-up front

# Runs just the native frontend
front:
	@echo "🚀 Initializing Frontend (Next.js)..."
	cd frontend && pnpm dev

# BACKEND (DOCKER)
# The backend and infrastructure are deployed via Docker (in the background).
back-up:
	@echo "🐳 Setting up the backend and infrastructure (Docker)..."
	cd backend && docker compose --profile dev up --build -d

# Knock down all the containers in the project.
back-down:
	@echo "🛑 Stopping containers..."
	cd backend && docker compose --profile dev down

# Monitor backend logs in real time.
back-logs:
	@echo "📋 Reading Backend Logs (Ctrl+C to exit)..."
	cd backend && docker compose --profile dev logs -f

# DATABASE BACKUP
# Defaults target the prod profile, since that is where the data that matters
# lives. Override with PROFILE=dev to exercise the same scripts locally.
PROFILE ?= prod
BACKUP_DIR ?= /var/backups/m-80

# Dump now. The systemd timer runs this same script daily - use it by hand
# before a deploy or a risky migration.
db-backup:
	@PROFILE=$(PROFILE) BACKUP_DIR=$(BACKUP_DIR) backend/scripts/db-backup.sh

db-backup-list:
	@ls -lh $(BACKUP_DIR)/m80-*.dump 2>/dev/null || echo "Nenhum dump em $(BACKUP_DIR)"

# Restores into a scratch database by default; MODE=overwrite replaces the live one.
db-restore:
	@test -n "$(FILE)" || { echo "Uso: make db-restore FILE=<caminho do dump> [MODE=overwrite]"; exit 1; }
	@PROFILE=$(PROFILE) backend/scripts/db-restore.sh $(if $(filter overwrite,$(MODE)),--overwrite,) $(FILE)

# Turns a production dump into one that can be loaded outside production.
#
# Its own profile variable, defaulting to dev: PROFILE above defaults to prod
# because that is where backups matter, and this is the one script that must
# never be aimed there - it creates a scratch copy of the database it runs
# against. See the script header for the two keys it requires.
ANON_PROFILE ?= dev

db-anonymize:
	@test -n "$(FILE)" || { echo "Uso: ENCRYPTION_KEY=... CPF_INDEX_KEY=... make db-anonymize FILE=<caminho do dump> [OUT=<destino>]"; exit 1; }
	@PROFILE=$(ANON_PROFILE) backend/scripts/db-anonymize.sh $(if $(OUT),--out $(OUT),) $(FILE)

# BROWSER EXTENSION (WXT)
# Dev build with hot reload, written to extension/.output/chrome-mv3-dev
ext-dev:
	cd extension && pnpm install && pnpm dev

# Production builds for both browsers, in extension/.output/{chrome,firefox}-mv3
ext-build:
	cd extension && pnpm install && pnpm test && pnpm build && pnpm build:firefox

# UTILS
# Clears caches and builds (does not delete Docker volumes to avoid data loss)
clean:
	@echo "🧹 Clearing frontend caches..."
	rm -rf frontend/.next
	rm -rf frontend/node_modules
	@echo "✨ Cleaning completed!"