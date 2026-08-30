# Backup e restauração do banco

O PostgreSQL do M-80 guarda **tudo**: emissões, passageiros, voos, vendas, clientes, contas de
fidelidade (com senha reversível), logs de auditoria e — o que costuma surpreender — **a sessão
pareada do WhatsApp**, que o whatsmeow mantém no mesmo banco. Perder o volume `db_prod_data` é
perder a operação inteira e ainda ter que reparear o bot.

Este documento é o runbook: o que roda sozinho, como instalar, e as duas receitas de recuperação.

## O que roda sozinho

Um systemd timer dispara `backend/scripts/db-backup.sh` **todo dia às 03:30** na máquina de
produção.

| | |
| --- | --- |
| Formato | `pg_dump -Fc` (custom, já comprimido) |
| Destino | `/var/backups/m-80/m80-AAAA-MM-DD_HHMM.dump` |
| Retenção | 14 dumps diários + o dump do dia 1º de cada mês, por 12 meses |
| Permissões | diretório `0700`, arquivos `0600` |
| Tamanho | ~650 KB por dump no volume de dados atual; a retenção inteira fica na casa de dezenas de MB |

Três detalhes do desenho que valem saber:

- **O `pg_dump` roda dentro do container do banco.** O client é sempre exatamente a versão do
  servidor — divergência entre os dois é a forma mais comum de um backup de Postgres apodrecer em
  silêncio — e a máquina não precisa de nenhuma ferramenta do Postgres instalada.
- **Todo dump é relido antes de ser aceito.** O script passa o arquivo pelo `pg_restore --list`; se
  ele estiver truncado ou corrompido, o arquivo é descartado e o script sai com erro, o que faz o
  systemd marcar a unidade como `failed`. Um backup que ninguém nunca leu não é backup.
- **A retenção lê a data do nome do arquivo, não o mtime.** Copiar ou tocar um dump não muda a idade
  que ele representa.

## Instalar na máquina

As units estão em `deploy/systemd/`. Elas assumem o clone em `/opt/m80` e o usuário
`deploy` — **ajuste `User`, `WorkingDirectory` e `ExecStart` no `.service` antes de copiar** se o seu
for diferente. O usuário precisa estar no grupo `docker`.

O diretório de destino precisa existir **com dono `deploy`** antes do primeiro dump. O script faz
`install -d -m 700` nele, mas roda sem privilégio e `/var/backups` é `root:root 755`:

```bash
sudo install -d -o deploy -g deploy -m 700 /var/backups/m-80
```

```bash
sudo cp deploy/systemd/m80-backup.service /etc/systemd/system/
sudo cp deploy/systemd/m80-backup.timer   /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now m80-backup.timer
```

Confira antes de confiar:

```bash
# Roda uma vez agora, sem esperar as 03:30
sudo systemctl start m80-backup.service
journalctl -u m80-backup -n 20         # deve terminar com "ok: /var/backups/m-80/..."
sudo ls -lh /var/backups/m-80          # o arquivo tem que estar lá, com tamanho plausível
systemctl list-timers m80-backup       # quando é o próximo disparo
```

Se a máquina estiver desligada às 03:30, o `Persistent=true` faz o dump rodar assim que ela
voltar, em
vez de pular o dia.

## Rodar um backup à mão

Antes de um deploy ou de uma migration arriscada:

```bash
make db-backup
make db-backup-list
```

Um `flock` impede que esse comando manual e o timer se atropelem — o segundo a chegar recusa em vez
de gerar um arquivo pela metade.

---

## Receita 1 — recuperar alguns registros *(o caso comum)*

Alguém apagou três emissões por engano. **Não restaure o banco inteiro por causa disso**: tudo que
foi feito depois do dump seria perdido junto. Restaure num banco paralelo e copie só o que importa.

**1. Abra o dump num banco de inspeção.** Produção não é tocada:

```bash
make db-restore FILE=/var/backups/m-80/m80-2026-08-05_0330.dump
```

O script cria `m80_restore_2026_08_05_0330` na mesma instância e imprime os comandos para abrir e
para apagar depois.

**2. Ache o que você quer de volta:**

```bash
docker compose --profile prod exec db-prod psql -U postgres -d m80_restore_2026_08_05_0330 \
  -c "SELECT id, program_locator, created_at FROM issuances WHERE created_at::date = '2026-08-04';"
```

**3. Copie as linhas de volta para produção.** `COPY ... TO STDOUT` de um banco direto para o outro,
sem arquivo intermediário e sem extensão nenhuma:

```bash
ID=361428b0-1a87-4d85-8fce-7713a7adf08b
SRC=m80_restore_2026_08_05_0330

copy_back() {
  docker compose --profile prod exec -T db-prod sh -c \
    "psql -U postgres -d $SRC -qtAc \"COPY (SELECT * FROM $1 WHERE $2) TO STDOUT\" \
     | psql -U postgres -d m80 -q -v ON_ERROR_STOP=1 -c 'COPY $1 FROM STDIN'"
}

# A ordem importa: as chaves estrangeiras exigem o pai antes do filho.
copy_back issuances                   "id='$ID'"
copy_back issuance_passengers         "issuance_id='$ID'"
copy_back issuance_flights            "issuance_id='$ID'"
copy_back issuance_passenger_flights  "passenger_id IN (SELECT id FROM issuance_passengers WHERE issuance_id='$ID')"
copy_back sales                       "issuance_id='$ID'"
copy_back audit_logs                  "issuance_id='$ID'"
```

Uma emissão nunca vem sozinha: `issuance_passengers` e `issuance_flights` são apagados em cascata
com ela, e `sales` e `audit_logs` a referenciam com `RESTRICT`. Recuperar só a linha de `issuances`
deixa a emissão sem passageiros e sem venda.

**4. Apague o banco de inspeção:**

```bash
docker compose --profile prod exec db-prod dropdb -U postgres m80_restore_2026_08_05_0330
```

---

## Receita 2 — restaurar tudo *(desastre)*

Só quando o banco inteiro se perdeu ou ficou inconsistente. **Tudo que foi escrito depois do dump
desaparece.**

```bash
make db-restore FILE=/var/backups/m-80/m80-2026-08-05_0330.dump MODE=overwrite
```

O script pede que você digite o nome do banco para confirmar, para `api-prod` e `whatsapp-bot-prod`
antes de começar (nada pode escrever durante a restauração), restaura numa única transação — ou o
banco fica inteiro no estado do dump, ou fica como estava, nunca com metade das tabelas dropadas — e
sobe os serviços de volta no fim, mesmo se o `pg_restore` falhar.

Duas consequências que não são óbvias:

- **A sessão do WhatsApp volta ao estado do dump.** Com um dump recente o bot reconecta sozinho
  (testado: `✅ Bot reconnected with the saved session!`). Com um dump antigo, pode ser preciso
  parear de novo pelo painel.
- **A tabela `schema_migrations` volta junto.** Se o dump for anterior a uma migration que já subiu,
  o golang-migrate reaplica as faltantes sozinho no próximo boot da API. É o comportamento desejado
  — só não se assuste ao ver migrations rodando depois de uma restauração.

Se o banco a restaurar estiver num volume novo (máquina reinstalada), suba só o `db-prod` primeiro,
rode a restauração e depois suba o resto.

---

## Limitações

**O dump mora no mesmo disco do banco.** Isto protege contra erro humano, deleção acidental e
corrupção de dados — que é de longe o que mais acontece. **Não** protege contra perder a
máquina: se o boot volume morrer ou a instância sumir, os backups vão junto. Isso ficou mais
importante depois da migração de 29/08/2026 — enquanto a VPS antiga existiu, ela era uma cópia
acidental do estado anterior; agora não há nenhuma.

Quando quiserem cópia externa, o gancho é o fim do `db-backup.sh`, depois do `mv` que promove o
`.part` a dump válido: um `rclone copy "$target" remoto:m-80/` ou `scp "$target" outra-maquina:` de
uma linha resolve. O arquivo já está verificado nesse ponto. O destino natural são os **20 GB de
Object Storage** que o Always Free da Oracle inclui — sobra muito para dumps de 1,3 MB.

**Os dumps contêm dados sensíveis** — senhas de conta de fidelidade (criptografia reversível) e
documentos de passageiro. O script cria o diretório `0700` e os arquivos `0600`; mantenha assim, e
não copie dump para lugar nenhum sem pensar nisso primeiro.
