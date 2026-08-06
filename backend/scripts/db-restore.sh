#!/usr/bin/env bash
#
# Restauração de um dump gerado por db-backup.sh.
#
# Dois modos, e o seguro é o padrão:
#
#   inspeção (padrão)  restaura num banco paralelo (m80_restore_<data>) na mesma
#                      instância. Produção não é tocada. É daqui que se recupera
#                      um punhado de registros apagados por engano.
#
#   --overwrite        restaura por cima do banco vivo, apagando o conteúdo
#                      atual. Para a API e o bot antes e sobe de volta no fim.
#
# Uso:
#   ./db-restore.sh /var/backups/m-80/m80-2026-08-05_0330.dump
#   ./db-restore.sh --overwrite /var/backups/m-80/m80-2026-08-05_0330.dump
#   PROFILE=dev ./db-restore.sh /tmp/m80-backups/m80-2026-08-05_2210.dump

set -Eeuo pipefail

PROFILE="${PROFILE:-prod}"
BACKEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICE="db-${PROFILE}"

MODE="inspect"
ASSUME_YES="false"
DUMP_FILE=""

log() { printf '%s  %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"; }
die() { printf '%s  ERRO: %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >&2; exit 1; }

# O cabeçalho de comentário do próprio arquivo é o texto de ajuda, para os dois
# nunca divergirem.
usage() {
  awk 'NR == 1 { next } /^#/ { sub(/^# ?/, ""); print; next } { exit }' "${BASH_SOURCE[0]}"
  exit "${1:-0}"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --overwrite) MODE="overwrite" ;;
    --yes|-y)    ASSUME_YES="true" ;;
    -h|--help)   usage 0 ;;
    -*)          die "opção desconhecida: $1" ;;
    *)           [ -z "$DUMP_FILE" ] || die "informe apenas um arquivo"; DUMP_FILE="$1" ;;
  esac
  shift
done

[ -n "$DUMP_FILE" ] || usage 1
[ -f "$DUMP_FILE" ] || die "arquivo não encontrado: $DUMP_FILE"
DUMP_FILE="$(cd "$(dirname "$DUMP_FILE")" && pwd)/$(basename "$DUMP_FILE")"

cd "$BACKEND_DIR"

compose() { docker compose --profile "$PROFILE" "$@"; }

# `docker compose exec -T` lê o stdin do processo. Todo exec que não precisa de
# entrada leva </dev/null, senão ele engole o que foi digitado (ou pipeado) para
# a confirmação mais abaixo.
running_services() { compose ps --status running --services 2>/dev/null </dev/null; }

running_services | grep -qx "$SERVICE" \
  || die "o serviço ${SERVICE} não está rodando"

# As credenciais vêm de dentro do container, das mesmas variáveis que o Compose
# usou para criar o banco — assim o script não pode divergir do .env.
DB_USER="$(compose exec -T "$SERVICE" printenv POSTGRES_USER </dev/null | tr -d '\r')"
DB_NAME="$(compose exec -T "$SERVICE" printenv POSTGRES_DB </dev/null | tr -d '\r')"

log "verificando o dump..."
compose exec -T "$SERVICE" pg_restore --list < "$DUMP_FILE" > /dev/null \
  || die "o arquivo não é um dump válido do pg_dump -Fc"

if [ "$MODE" = "inspect" ]; then
  stamp="$(basename "$DUMP_FILE" .dump)"
  stamp="${stamp#m80-}"
  scratch_db="m80_restore_$(printf '%s' "$stamp" | tr -c 'a-zA-Z0-9' '_')"

  log "criando banco de inspeção ${scratch_db}..."
  compose exec -T -e SCRATCH_DB="$scratch_db" "$SERVICE" \
    sh -c 'createdb -U "$POSTGRES_USER" "$SCRATCH_DB"' </dev/null \
    || die "não consegui criar ${scratch_db} (já existe? apague com: dropdb -U ${DB_USER} ${scratch_db})"

  log "restaurando..."
  # Sem --exit-on-error: aviso de owner/privilégio num banco novo é esperado e
  # não invalida os dados. O status é reportado, não engolido.
  if ! compose exec -T -e SCRATCH_DB="$scratch_db" "$SERVICE" \
      sh -c 'pg_restore -U "$POSTGRES_USER" -d "$SCRATCH_DB" --no-owner --no-privileges' < "$DUMP_FILE"; then
    log "aviso: o pg_restore relatou erros — confira os dados antes de confiar neles"
  fi

  cat <<EOF

Pronto. O banco de produção não foi tocado.

  Abrir:   docker compose --profile ${PROFILE} exec ${SERVICE} psql -U ${DB_USER} -d ${scratch_db}
  Apagar:  docker compose --profile ${PROFILE} exec ${SERVICE} dropdb -U ${DB_USER} ${scratch_db}

Para copiar registros de volta para ${DB_NAME}: COPY de um banco para o outro,
respeitando a ordem das chaves estrangeiras. Receita pronta em docs/backup.md.
EOF
  exit 0
fi

# --- overwrite ---------------------------------------------------------------

cat <<EOF

⚠️  RESTAURAÇÃO TOTAL sobre o banco "${DB_NAME}" (perfil ${PROFILE}).

    Todo o conteúdo atual será apagado e substituído pelo dump:
      $(basename "$DUMP_FILE")

    A sessão pareada do WhatsApp também volta ao estado do dump (o whatsmeow
    guarda no mesmo banco). Com um dump recente o bot reconecta sozinho; com um
    dump antigo pode ser preciso parear de novo.

EOF

if [ "$ASSUME_YES" != "true" ]; then
  printf 'Digite o nome do banco para confirmar: '
  read -r confirmation \
    || die "não recebi confirmação (stdin não é interativo) — use --yes se for intencional"
  [ "$confirmation" = "$DB_NAME" ] || die "confirmação não confere — nada foi alterado"
fi

app_services=()
for candidate in "api-${PROFILE}" "whatsapp-bot-${PROFILE}"; do
  if running_services | grep -qx "$candidate"; then
    app_services+=("$candidate")
  fi
done

restart_apps() {
  [ ${#app_services[@]} -gt 0 ] || return 0
  log "subindo de volta: ${app_services[*]}"
  compose start "${app_services[@]}" || log "aviso: não consegui subir ${app_services[*]} — suba à mão"
}

if [ ${#app_services[@]} -gt 0 ]; then
  log "parando ${app_services[*]} para que nada escreva durante a restauração..."
  compose stop "${app_services[@]}"
  # Mesmo se o pg_restore falhar, a aplicação tem que voltar.
  trap restart_apps EXIT
fi

log "restaurando ${DB_NAME}..."
# --single-transaction: ou o banco fica inteiro no estado do dump, ou fica como
# estava. Não existe meio-termo com metade das tabelas dropadas.
compose exec -T "$SERVICE" \
  sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --no-privileges --single-transaction' \
  < "$DUMP_FILE" \
  || die "pg_restore falhou — o banco continua no estado anterior"

log "ok: ${DB_NAME} restaurado a partir de $(basename "$DUMP_FILE")"
