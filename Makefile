.PHONY: dev front back-up back-down back-logs clean

# LOCAL DEVELOPMENT (DOCKER-FIRST)

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

# UTILS
# Clears caches and builds (does not delete Docker volumes to avoid data loss)
clean:
	@echo "🧹 Clearing frontend caches..."
	rm -rf frontend/.next
	rm -rf frontend/node_modules
	@echo "✨ Cleaning completed!"