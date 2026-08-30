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
    if ($7 != "") {                      # cpu_some_avg60, por instante
      if (!(($1) in seen)) { seen[$1] = 1; pr[++np] = $7 + 0 }
    }
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
    # Pressão é percentual de tempo em que ALGUMA tarefa esperou CPU. Poucos
    # por cento são normais numa máquina trabalhando; o sinal de escassez é o
    # valor ficar alto de forma sustentada, não ser diferente de zero. Por isso
    # aqui saem números e faixas, não um veredito de uma linha.
    printf "\npressão de CPU (some avg60, %% do tempo com alguma tarefa esperando)\n"
    if (np == 0) { print "  (sem dados de PSI)"; exit }
    asort(pr)
    over10 = 0; over20 = 0
    for (i = 1; i <= np; i++) { if (pr[i] > 10) over10++; if (pr[i] > 20) over20++ }
    printf "  mediana  %.2f%%\n", pr[int(np*0.50)+1]
    printf "  p95      %.2f%%\n", pr[int(np*0.95)+1]
    printf "  máximo   %.2f%%\n", pr[np]
    printf "  acima de 10%%: %.1f%% do tempo   acima de 20%%: %.1f%% do tempo\n", \
           100*over10/np, 100*over20/np
    print  "\ncomo ler: a decisão é o p95. Abaixo de ~10%% a máquina está trabalhando,"
    print  "não sofrendo. Sustentado acima de 20%% é escassez real e justifica 3 OCPU."
    print  "Janelas curtas ou que incluam um deploy não valem como veredito."
  }
' "${files[@]}"
