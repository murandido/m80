#!/usr/bin/env bash
#
# Dump do PostgreSQL do M-80.
#
# O pg_dump roda DENTRO do container do banco de propósito: assim o client é
# sempre exatamente a versão do servidor — divergência de versão entre os dois é
# a forma mais comum de um backup de Postgres apodrecer em silêncio — e o host
# não precisa de nenhuma ferramenta do Postgres instalada.
#
# Uso:
#   ./db-backup.sh
#   PROFILE=dev BACKUP_DIR=/tmp/m80-backups ./db-backup.sh
#
# Variáveis:
#   PROFILE              perfil do Compose (default: prod)
#   BACKUP_DIR           onde guardar os dumps (default: /var/backups/m-80)
#   DAILY_KEEP_DAYS      por quantos dias manter os dumps diários (default: 14)
#   MONTHLY_KEEP_MONTHS  por quantos meses manter os dumps do dia 1º (default: 12)

set -Eeuo pipefail

PROFILE="${PROFILE:-prod}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/m-80}"
DAILY_KEEP_DAYS="${DAILY_KEEP_DAYS:-14}"
MONTHLY_KEEP_MONTHS="${MONTHLY_KEEP_MONTHS:-12}"

BACKEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICE="db-${PROFILE}"

log() { printf '%s  %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"; }
die() { printf '%s  ERRO: %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >&2; exit 1; }

cd "$BACKEND_DIR"

compose() { docker compose --profile "$PROFILE" "$@"; }

# Retention runs on filenames, not mtime: copiar ou tocar um arquivo não pode
# mudar a idade que ele representa.
prune() {
  local now_epoch file base date_part day file_epoch age_days
  now_epoch="$(date +%s)"

  for file in "$BACKUP_DIR"/m80-*.dump; do
    [ -e "$file" ] || continue

    base="${file##*/}"
    date_part="${base#m80-}"
    date_part="${date_part%%_*}"
    day="${date_part##*-}"

    if ! file_epoch="$(date -d "$date_part" +%s 2>/dev/null)"; then
      log "aviso: ignorando ${base} (não consigo ler a data do nome)"
      continue
    fi

    age_days=$(( (now_epoch - file_epoch) / 86400 ))

    if [ "$day" = "01" ]; then
      # O dump do dia 1º é promovido a mensal e sobrevive à janela diária.
      if [ "$age_days" -gt $(( MONTHLY_KEEP_MONTHS * 31 )) ]; then
        rm -f -- "$file"
        log "removido (mensal expirado): ${base}"
      fi
    elif [ "$age_days" -gt "$DAILY_KEEP_DAYS" ]; then
      rm -f -- "$file"
      log "removido (diário expirado): ${base}"
    fi
  done
}

install -d -m 700 "$BACKUP_DIR"

# Impede que o timer e um `make db-backup` manual se atropelem. O fd fica aberto
# pelo resto do processo e o lock cai sozinho quando ele termina.
exec 9>"$BACKUP_DIR/.backup.lock"
flock -n 9 || die "outro backup já está em execução (lock em $BACKUP_DIR/.backup.lock)"

stamp="$(date +%Y-%m-%d_%H%M)"
target="$BACKUP_DIR/m80-${stamp}.dump"
part="${target}.part"

# Um .part sobrando significa dump interrompido — nunca deve virar backup válido.
trap 'rm -f -- "$part"' EXIT

umask 077

log "gerando dump do serviço ${SERVICE}..."
# -Fc (custom): já sai comprimido, permite restauração seletiva de tabela e é o
# único formato que o pg_restore aceita. Um .sql puro seria maior e só
# restaurável por inteiro.
# </dev/null porque `docker compose exec -T` lê o stdin do processo, e um dump
# não tem o que fazer com ele.
compose exec -T "$SERVICE" sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' </dev/null > "$part" \
  || die "pg_dump falhou"

[ -s "$part" ] || die "o dump saiu vazio"

# Um backup que nunca foi lido não é backup: relê o arquivo com o pg_restore do
# próprio container, que rejeita dump truncado ou corrompido.
log "verificando integridade do dump..."
compose exec -T "$SERVICE" pg_restore --list < "$part" > /dev/null \
  || die "o dump não passou na verificação do pg_restore — arquivo descartado"

mv -- "$part" "$target"
chmod 600 -- "$target"

prune

size="$(du -h -- "$target" | cut -f1)"
total="$(find "$BACKUP_DIR" -maxdepth 1 -name 'm80-*.dump' | wc -l)"
log "ok: ${target} (${size}) — ${total} dump(s) no diretório"
