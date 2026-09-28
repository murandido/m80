# CA's Mode

Um segundo ambiente completo — API, bot, promo, pdf-service, Postgres, painel e Caddy — numa VPS
Oracle separada, rodando a branch **`ca-mode`**. Existe para o designer olhar e usar telas que ainda
não foram decididas, sem que nada disso encoste em produção.

O nome vem do Cauê, que desenha as telas no Figma; o código é escrito e publicado aqui.

## O que ele é, e o que ele não é

**É** uma cópia fiel da produção: mesma stack, mesmo Compose, mesmo `deploy/vps-deploy.sh`, mesmo
runbook de provisionamento. Quanto mais parecido, mais o que se aprende ali vale para o que vai ao ar.

**Não é** um ambiente com dados de verdade, e **não é** um lugar seguro para experimentar migration
destrutiva achando que não tem consequência — ver *Migrations* no fim.

| | Produção | CA's Mode |
| --- | --- | --- |
| Branch | `main` | `ca-mode` |
| Label de deploy | `deploy:vps` | `deploy:ca` |
| Workflow | `.github/workflows/deploy-vps.yml` | `.github/workflows/deploy-ca.yml` |
| Tag flutuante | `latest` | `ca-latest` |
| Secrets | `VPS_*` | `CA_VPS_*` |
| Dados | reais | dump de produção anonimizado |
| Bot do WhatsApp | pareado | **nunca pareado** |

## Por que não existe um profile `staging` no Compose

Porque não precisa. O profile `prod` já resolve domínio, credenciais e chaves inteiramente a partir
do `.env` da máquina, e as duas VPSs são máquinas separadas — não há colisão de `container_name`, de
rede nem de volume. Duplicar sete serviços criaria um profile que envelhece em silêncio ao lado do de
produção, e o valor do ambiente está justamente em ele ser a mesma configuração.

O que separa os dois ambientes é o `.env` de cada máquina, e mais nada.

## A branch

`ca-mode` sai da `main` e vive. Trabalhe nela numa git worktree separada, para não trocar a branch
debaixo de outro trabalho em andamento:

```bash
git worktree add ../m-80-monorepo-ca-mode ca-mode
```

- Experimento → PR **para `ca-mode`** → merge → label `deploy:ca`.
- Rebase periódico sobre a `main`, para a divergência não virar um problema de uma vez só.
- O que vingar vira PR **de `ca-mode` para `main`**, revisado como qualquer feature.

PRs para `ca-mode` rodam o mesmo CI que os de `main` (`ci.yml`).

## Secrets do GitHub

Em **Settings → Secrets and variables → Actions**, os mesmos cinco da produção com prefixo `CA_`:

| Secret | Para quê | Obrigatório |
| --- | --- | --- |
| `CA_VPS_HOST` | IP ou hostname da máquina do CA's Mode | sim |
| `CA_VPS_USER` | usuário de deploy (`deploy`) | sim |
| `CA_VPS_SSH_KEY` | chave privada que o CI usa | sim |
| `CA_VPS_SSH_PORT` | porta do SSH (default `22`) | não |
| `CA_VPS_KNOWN_HOSTS` | host key fixada; sem ela, cai para `ssh-keyscan` | recomendado |

## Provisionamento

Siga `docs/deploy.md` — os treze passos são os mesmos, a máquina só é outra. O que muda:

1. **`IMAGE_TAG=ca-latest` no `.env`.** Não é detalhe: o Compose resolve `${IMAGE_TAG:-latest}`, e
   `latest` é a tag da **produção**. Sem essa linha, um `docker compose up` à mão nessa máquina sobe
   as imagens de produção contra o banco do CA's Mode. O deploy pela label não passa por aí (ele
   sempre nomeia o SHA), mas todo comando manual passa.
2. **`ENCRYPTION_KEY_PROD` e `CPF_INDEX_KEY_PROD` novas** (`openssl rand -hex 32` cada). Nunca as de
   produção — e são elas que o anonimizador recebe ao gerar o dump.
3. **`TRAVEOS_*_PROD` em branco.** A Traveos não tem sandbox: credencial real aqui bate no tenant de
   produção do fornecedor. Sem elas o bot sobe normalmente, apenas sem a integração.
4. **`API_DOMAIN_PROD="ca.emioitenta.com.br"`**, com os registros A de `ca.` **e** `www.ca.`
   resolvendo para esta máquina **antes** de subir o Caddy (o Let's Encrypt tem limite de 5 falhas
   por hora por domínio). O `www.ca.` não é opcional: o Caddyfile serve `www.{$API_DOMAIN}` para
   redirecionar, e sem o registro esse certificado falha em loop. Para recriar o Caddy à mão aqui:
   ```bash
   IMAGE_TAG=$(sed -n 's/^CURRENT_TAG=//p' /opt/m80/deploy/.deploy-state) \
     docker compose --profile prod up -d --force-recreate --no-deps caddy-prod
   ```
   Sem `--no-deps`, o Compose recria junto o painel e a API na tag que você passou — e `latest` /
   `ca-latest` na máquina podem ser imagens velhas, porque o deploy publica por SHA. Foi assim que a
   API de produção caiu em 28/09/2026: subiu uma imagem que não conhecia a migration do banco.
5. **PAT do GHCR** classic, escopo somente `read:packages`, `docker login ghcr.io` como `deploy`.
   Gere um sem prazo curto: um PAT de 30 dias vira uma falha de `pull` no meio de um deploy.
6. **`sudo install -d -o deploy -g deploy -m 700 /var/backups/m-80`.** Sem isso o **segundo** deploy
   morre no dump pré-deploy — o primeiro passa, porque o script pula o dump quando não há banco de pé.
7. **Clone da `ca-mode`** em `/opt/m80`, não da `main`.

## Os dados

Vêm de um dump de produção **anonimizado na sua máquina**. O dump bruto nunca vai para a outra VPS.

```bash
make db-backup                                  # na produção
scp <prod>:/var/backups/m-80/m80-<data>.dump .  # para a sua máquina
ENCRYPTION_KEY=<a do CA> CPF_INDEX_KEY=<a do CA> \
  make db-anonymize FILE=m80-<data>.dump        # gera m80-ca-<stamp>.dump
```

`backend/scripts/db-anonymize.sh` restaura o dump num banco descartável, roda `cmd/anonymize` por
cima e gera o segundo dump. O banco descartável é apagado ao fim, inclusive em caso de erro.

O anonimizador **nunca descriptografa nada**: ele descarta o valor real e escreve um valor falso já
criptografado com a chave de destino. A `ENCRYPTION_KEY` de produção não é necessária e não deve ser
fornecida. Ele também recusa rodar contra qualquer banco cujo nome não comece com `m80_anon_`, e não
existe flag para contornar isso.

O que ele preserva de propósito: **identidade, não valores**. Duas linhas que tinham o mesmo CPF
continuam com o mesmo CPF falso e o mesmo nome falso, porque tudo é derivado do blind index antigo.
É o que mantém pessoa, documento de fidelidade e beneficiário apontando um para o outro.

O que ele destrói: nomes, CPFs, documentos, telefones, e-mails, endereços, apelidos de pessoa, de
conta e de agência, nomes de agência, localizadores e números de compra (embaralhados no mesmo
formato), observações de emissão, notas de lote e de pagamento, os relatórios do promo (que citam
CPF em texto puro), `audit_logs`, `listened_chats` e **as tabelas `whatsmeow_*`**.

**Tabela nova com texto é tabela nova para o anonimizador.** Ele trata as colunas que conhece, e uma
coluna de nome ou de texto livre criada depois passa por ele intacta. Depois de gerar o dump, cruze
os valores reais do dump de origem com todas as colunas de texto do anonimizado antes de enviá-lo.
Foi assim que apelidos de conta, localizadores e relatórios do promo apareceram na primeira carga.

Depois, restaure na máquina do CA's Mode:

```bash
make db-restore FILE=m80-ca-<stamp>.dump MODE=overwrite
```

Todo usuário e toda conta de fidelidade ficam com a senha `ca-mode-1234`; o anonimizador imprime os
telefones de login ao terminar.

## O bot fica despareado, e isso é o ponto

O whatsmeow guarda a sessão do WhatsApp **no mesmo Postgres** da aplicação (`backend/cmd/bot/main.go`).
Um dump de produção restaurado em outra máquina com essas tabelas intactas faz o bot de lá acordar
pareado com o WhatsApp real do negócio: dois bots lendo as conversas dos mesmos clientes, os dois
capazes de responder e de criar emissão.

Por isso o anonimizador dropa as tabelas. O whatsmeow as recria vazias no boot, `GetFirstDevice` não
acha nada, e o bot fica esperando pareamento para sempre. O painel funciona inteiro; só não chega
mensagem.

**Se um dia esse ambiente precisar de um bot que funcione, pareie com um chip separado** — nunca
restaurando a sessão de produção.

## Migrations

Este é o ponto de atenção do fluxo, e o único lugar onde o CA's Mode pode machucar a produção.

Migration roda no boot da API e é a única parte de um deploy que o rollback **não** desfaz — nos dois
ambientes. Uma migration criada na `ca-mode` fica "à frente" da `main`, e ao voltar por PR precisa ser
renumerada se a `main` ganhou migrations no meio-tempo. **Confira o último número dos dois lados
antes de criar uma.** Duas migrations com o mesmo número são um conflito que só aparece no deploy.

## Verificar que os dois ambientes não se cruzam

Vale fazer uma vez, e refazer se algum dia os workflows forem mexidos:

- Label `deploy:ca` num PR com base `main` → **nada acontece**.
- Label `deploy:vps` num PR com base `ca-mode` → **nada acontece**.

As duas coisas são garantidas pela condição de `base.ref` no `if:` de cada workflow, não pelo nome da
label.
