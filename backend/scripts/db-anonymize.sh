#!/usr/bin/env bash
#
# Transforma um dump de produção num dump carregável fora da produção.
#
# Restaura o dump num banco descartável, roda cmd/anonymize por cima dele e
# gera um segundo dump — já sem nome, CPF, documento, telefone, observação e
# sem a sessão pareada do WhatsApp. O dump de entrada não é alterado.
#
# Roda na SUA máquina, contra o Postgres do perfil dev. Nunca na produção: o
# banco descartável nasce na mesma instância em que o script roda, e é a última
# coisa que se quer criar num servidor que está atendendo gente.
#
# As chaves do ambiente de DESTINO são obrigatórias, e não podem ser as de
# produção — é com elas que os valores falsos são criptografados:
#
#   ENCRYPTION_KEY=$(openssl rand -hex 32)
#   CPF_INDEX_KEY=$(openssl rand -hex 32)
#
# Uso:
#   ENCRYPTION_KEY=... CPF_INDEX_KEY=... ./db-anonymize.sh m80-2026-09-05_0330.dump
#   ... ./db-anonymize.sh --out /tmp/ca.dump m80-2026-09-05_0330.dump

set -Eeuo pipefail

# Diferente dos outros scripts daqui, o padrão é dev: este é um utilitário de
# máquina local, e apontá-lo para prod é justamente o que ele existe para evitar.
PROFILE="${PROFILE:-dev}"
BACKEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICE="db-${PROFILE}"

# O anonimizador roda no host e fala com o banco pela porta que o Compose
# publica (o perfil dev publica 5432).
ANON_DB_HOST="${ANON_DB_HOST:-localhost}"
ANON_DB_PORT="${ANON_DB_PORT:-5432}"

DUMP_FILE=""
OUT_FILE=""

log() { printf '%s  %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"; }
die() { printf '%s  ERRO: %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >&2; exit 1; }

usage() {
  awk 'NR == 1 { next } /^#/ { sub(/^# ?/, ""); print; next } { exit }' "${BASH_SOURCE[0]}"
  exit "${1:-0}"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --out|-o)  shift; [ $# -gt 0 ] || die "--out exige um caminho"; OUT_FILE="$1" ;;
    -h|--help) usage 0 ;;
    -*)        die "opção desconhecida: $1" ;;
    *)         [ -z "$DUMP_FILE" ] || die "informe apenas um arquivo"; DUMP_FILE="$1" ;;
  esac
  shift
done

[ -n "$DUMP_FILE" ] || usage 1
[ -f "$DUMP_FILE" ] || die "arquivo não encontrado: $DUMP_FILE"
DUMP_FILE="$(cd "$(dirname "$DUMP_FILE")" && pwd)/$(basename "$DUMP_FILE")"

[ -n "${ENCRYPTION_KEY:-}" ] || die "ENCRYPTION_KEY não definida (openssl rand -hex 32)"
[ -n "${CPF_INDEX_KEY:-}" ]  || die "CPF_INDEX_KEY não definida (openssl rand -hex 32)"
[ "$ENCRYPTION_KEY" != "$CPF_INDEX_KEY" ] || die "ENCRYPTION_KEY e CPF_INDEX_KEY têm que ser chaves diferentes"

command -v go >/dev/null || die "go não encontrado no PATH (o anonimizador roda no host)"

cd "$BACKEND_DIR"

compose() { docker compose --profile "$PROFILE" "$@"; }
running_services() { compose ps --status running --services 2>/dev/null </dev/null; }

running_services | grep -qx "$SERVICE" || die "o serviço ${SERVICE} não está rodando (make back-up)"

DB_USER="$(compose exec -T "$SERVICE" printenv POSTGRES_USER </dev/null | tr -d '\r')"
DB_PASSWORD="$(compose exec -T "$SERVICE" printenv POSTGRES_PASSWORD </dev/null | tr -d '\r')"

log "verificando o dump..."
compose exec -T "$SERVICE" pg_restore --list < "$DUMP_FILE" > /dev/null \
  || die "o arquivo não é um dump válido do pg_dump -Fc"

# O prefixo m80_anon_ não é cosmético: cmd/anonymize recusa rodar contra
# qualquer banco cujo nome não comece com ele, e não há flag para contornar.
stamp="$(date '+%Y%m%d_%H%M%S')"
scratch_db="m80_anon_${stamp}"
[ -n "$OUT_FILE" ] || OUT_FILE="$(dirname "$DUMP_FILE")/m80-ca-${stamp}.dump"

# O banco descartável some em qualquer saída, inclusive erro: ele é uma cópia
# não anonimizada da produção, e não deve sobreviver ao script.
cleanup() {
  log "removendo ${scratch_db}..."
  compose exec -T -e SCRATCH_DB="$scratch_db" "$SERVICE" \
    sh -c 'dropdb -U "$POSTGRES_USER" --if-exists --force "$SCRATCH_DB"' </dev/null \
    || log "aviso: não consegui remover ${scratch_db} — apague à mão, ele contém dados reais"
}

log "criando banco descartável ${scratch_db}..."
compose exec -T -e SCRATCH_DB="$scratch_db" "$SERVICE" \
  sh -c 'createdb -U "$POSTGRES_USER" "$SCRATCH_DB"' </dev/null \
  || die "não consegui criar ${scratch_db}"
trap cleanup EXIT

log "restaurando o dump..."
if ! compose exec -T -e SCRATCH_DB="$scratch_db" "$SERVICE" \
    sh -c 'pg_restore -U "$POSTGRES_USER" -d "$SCRATCH_DB" --no-owner --no-privileges' < "$DUMP_FILE"; then
  log "aviso: o pg_restore relatou erros (owner/privilégio num banco novo é esperado)"
fi

log "anonimizando..."
DB_HOST="$ANON_DB_HOST" \
DB_PORT="$ANON_DB_PORT" \
DB_USER="$DB_USER" \
DB_PASSWORD="$DB_PASSWORD" \
DB_NAME="$scratch_db" \
ENCRYPTION_KEY="$ENCRYPTION_KEY" \
CPF_INDEX_KEY="$CPF_INDEX_KEY" \
  go run ./cmd/anonymize || die "a anonimização falhou — nada foi gravado"

# Gravado como .part e promovido só no fim, para um erro aqui não deixar para
# trás um arquivo truncado com cara de dump pronto.
log "gerando ${OUT_FILE}..."
compose exec -T -e SCRATCH_DB="$scratch_db" "$SERVICE" \
  sh -c 'pg_dump -U "$POSTGRES_USER" -Fc "$SCRATCH_DB"' </dev/null > "${OUT_FILE}.part" \
  || die "pg_dump falhou"

compose exec -T "$SERVICE" pg_restore --list < "${OUT_FILE}.part" > /dev/null \
  || die "o dump gerado não passou na releitura — descartado"

mv "${OUT_FILE}.part" "$OUT_FILE"
chmod 600 "$OUT_FILE"

cat <<EOF

Pronto: ${OUT_FILE}

O dump de origem não foi alterado e o banco descartável já foi removido.

Antes de subir isto para qualquer lugar, confira você mesmo — é o único passo
que separa um dump anonimizado de um vazamento:

  docker compose --profile ${PROFILE} exec ${SERVICE} psql -U ${DB_USER} -d <banco> \\
    -c 'SELECT name FROM people LIMIT 5;' \\
    -c "SELECT tablename FROM pg_tables WHERE tablename LIKE 'whatsmeow%';"
EOF
