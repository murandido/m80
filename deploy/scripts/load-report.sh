#!/usr/bin/env bash
#
# Lê os CSVs do load-sample.sh e responde as três perguntas que decidem o shape:
#
#   1. p95 de CPU da máquina  -> 2 OCPU bastam?
#   2. pico por container     -> quem causa o pico (a aposta é o promo)
#   3. pressão de CPU nos picos -> faltou CPU de verdade, ou só houve uso?
#
# A diferença entre 2 e 3 é o ponto: utilização alta com pressão zero é uma
# máquina trabalhando: utilização alta com pressão sustentada é uma máquina
# curta. Só a segunda justifica pagar por mais OCPU.

set -euo pipefail

OUT_DIR="${OUT_DIR:-/var/log/m80}"
files=("$OUT_DIR"/load-*.csv)
[ -e "${files[0]}" ] || { echo "sem amostras em $OUT_DIR" >&2; exit 1; }

awk -F, '
  FNR == 1 { next }                      # cabeçalho de cada arquivo
  {
    cpu[$1] += $3                        # soma o CPUPerc de todos os containers no mesmo instante
    if ($3 > peak[$2]) peak[$2] = $3
    if ($7 != "") { p[++np] = $7 + 0 }   # cpu_some_avg60
    if ($7 + 0 > 0) stalls++
    samples++
  }
  END {
    n = 0; for (t in cpu) tot[n++] = cpu[t]
    asort(tot)
    printf "amostras: %d   instantes: %d\n\n", samples, n
    printf "CPU da máquina (soma dos containers, %% de UM core; 200%% = dois cores saturados)\n"
    printf "  mediana  %.1f%%\n", tot[int(n*0.50)+1]
    printf "  p95      %.1f%%\n", tot[int(n*0.95)+1]
    printf "  máximo   %.1f%%\n\n", tot[n]
    printf "pico por container\n"
    m = 0; for (c in peak) if (peak[c] > 0) { printf "  %-24s %.1f%%\n", c, peak[c]; m++ }
    if (m == 0) print "  (nenhum acima de zero)"
    printf "\npressão de CPU (some avg60): %d de %d amostras acima de zero\n", stalls+0, samples
    if (stalls+0 == 0) print "  -> nenhuma tarefa esperou CPU. Sobra capacidade."
    else print "  -> houve espera por CPU. Avalie subir para 3 OCPU."
  }
' "${files[@]}"
