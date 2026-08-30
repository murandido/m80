#!/usr/bin/env bash
#
# Amostra a carga da máquina para dimensionar a instância.
#
# A pergunta que este script existe para responder é "2 OCPU bastam?", e o
# painel do provedor não responde: ele agrega em janelas de minutos, e o pico
# que importa aqui - um render do Chrome no promo, um PDF no WeasyPrint - dura
# segundos. Diluído numa média de 5 minutos, um core saturado vira 5%.
#
# Grava três coisas por amostra, e a terceira é a que decide:
#
#   docker stats   quanto cada container consome. Em máquina multi-core o
#                  CPUPerc passa de 100%: 200% são os dois cores saturados.
#   pressure/cpu   quanto tempo alguma tarefa ficou *esperando* CPU. Zero
#                  durante os picos significa que sobrou; valor sustentado
#                  significa que faltou. Utilização não distingue os dois.
#   pressure/memory  o mesmo para memória.
#
# O p95 de CPU tem um segundo uso: é a métrica que a Oracle usa para reclamar
# instância Always Free ociosa (p95 de CPU, rede e memória todos abaixo de 20%
# por 7 dias). Numa conta Pay As You Go isso não se aplica, mas o número
# continua sendo o mesmo.
#
# Roda como serviço contínuo, não como timer: amostragem de 2s não é um job
# que começa e termina.

set -uo pipefail

INTERVAL="${INTERVAL:-2}"
OUT_DIR="${OUT_DIR:-/var/log/m80}"
KEEP_DAYS="${KEEP_DAYS:-30}"

mkdir -p "$OUT_DIR"

current_file() { printf '%s/load-%s.csv' "$OUT_DIR" "$(date +%F)"; }

# O cabeçalho é escrito uma vez por arquivo, para o CSV do dia abrir direto em
# qualquer ferramenta sem tratamento.
ensure_header() {
  local f="$1"
  [ -s "$f" ] || printf 'ts,container,cpu_pct,mem_used,mem_limit,cpu_some_avg10,cpu_some_avg60,mem_some_avg60\n' > "$f"
}

# some avg10/avg60 da pressão de CPU e o avg60 da de memória. Ausentes em kernel
# sem PSI, e nesse caso ficam vazios em vez de derrubar a amostragem.
read_pressure() {
  local c10="" c60="" m60=""
  # split no '=' em vez de tr: tr -d apaga um CONJUNTO de caracteres, não um
  # prefixo, então 'tr -d "avg60="' comia os dígitos 0, 6 e 1 de dentro do
  # próprio valor - 0.06 virava '.'. Silencioso, e justamente na coluna que
  # decide se faltou CPU.
  if [ -r /proc/pressure/cpu ]; then
    read -r c10 c60 < <(awk '/^some/{split($2,a,"="); split($3,b,"="); print a[2], b[2]}' /proc/pressure/cpu)
  fi
  if [ -r /proc/pressure/memory ]; then
    m60=$(awk '/^some/{split($3,b,"="); print b[2]}' /proc/pressure/memory)
  fi
  printf '%s,%s,%s' "${c10:-}" "${c60:-}" "${m60:-}"
}

trap 'exit 0' TERM INT

last_prune=""
while :; do
  f="$(current_file)"
  ensure_header "$f"
  ts="$(date -Is)"
  pressure="$(read_pressure)"

  # --no-stream faz uma leitura instantânea; sem ele o docker stats fica preso
  # num loop próprio e o intervalo aqui deixa de valer.
  docker stats --no-stream --format '{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}' 2>/dev/null \
    | while IFS=$'\t' read -r name cpu mem; do
        used="${mem%% /*}"
        limit="${mem##*/ }"
        printf '%s,%s,%s,%s,%s,%s\n' \
          "$ts" "$name" "${cpu%\%}" "$used" "$limit" "$pressure" >> "$f"
      done

  # Poda uma vez por dia, não a cada amostra.
  today="$(date +%F)"
  if [ "$last_prune" != "$today" ]; then
    find "$OUT_DIR" -name 'load-*.csv' -type f -mtime "+$KEEP_DAYS" -delete 2>/dev/null
    last_prune="$today"
  fi

  sleep "$INTERVAL"
done
