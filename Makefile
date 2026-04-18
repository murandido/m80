.PHONY: dev front back-up back-down back-logs clean

# LOCAL DEVELOPMENT (DOCKER-FIRST)

# Master Command: Backend setup and run native frontend
dev: back-up front

# Runs just the native frontend
front:
	@echo "🚀 Initializing Frontend (Next.js)..."
	cd frontend && pnpm dev

# ==============================================================================
# BACKEND (DOCKER)
# ==============================================================================

# Sobe o backend e infraestrutura via Docker (em background)
back-up:
	@echo "🐳 Setting up the backend and infrastructure (Docker)..."
	cd backend && docker compose --profile dev up --build -d

# Derruba todos os containers do projeto
back-down:
	@echo "🛑 Stopping containers..."
	cd backend && docker compose --profile dev down

# Acompanha os logs do backend em tempo real
back-logs:
	@echo "📋 Reading Backend Logs (Ctrl+C to exit)..."
	cd backend && docker compose --profile dev logs -f

# ==============================================================================
# UTILS
# ==============================================================================

# Limpa caches e builds (não apaga os volumes do Docker para não perder dados)
clean:
	@echo "🧹 Clearing frontend caches..."
	rm -rf frontend/.next
	rm -rf frontend/node_modules
	@echo "✨ Cleaning completed!"