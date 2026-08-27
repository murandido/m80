# Deploy

O M-80 vai ao ar em dois lugares independentes: a **API, o bot, o promo-service e o pdf-service** rodam em Docker
numa VPS da Hostinger, atrás do Caddy; o **painel** roda na Vercel. Os dois são publicados por
**label no GitHub**, nunca por push ou merge.

Este documento é o runbook: como publicar, como provisionar do zero, e o que fazer quando dá errado.

## Merge não é publicação

`main` nem sempre é versão final. Por isso mergear e publicar são dois atos separados:

1. Mergeie o PR normalmente (squash merge, como sempre).
2. Quando aquilo tiver que ir ao ar, aplique a label no PR **já mergeado**:

| Label | Publica |
| --- | --- |
| `deploy:vps` | API + bot + promo-service + pdf-service na VPS (`.github/workflows/deploy-vps.yml`) |
| `deploy:vercel` | painel na Vercel (`.github/workflows/deploy-vercel.yml`) |

As duas são independentes — uma mudança só de frontend não precisa mexer na VPS.

**Para republicar a mesma versão**, tire e recoloque a label: o gatilho é o evento `labeled`, então
uma label que já está lá não dispara nada.

**O que vai ao ar é a `main`, não o PR.** Como o PR já foi mergeado, o primeiro job resolve o SHA da
`main` e todos os outros se fixam nele. Se alguém der push na main no meio do deploy, o que subiu
continua sendo um commit só, inteiro.

**Na primeira vez tem uma pegadinha:** o workflow precisa já estar na `main` para uma label disparar.
O próprio PR que introduz este pipeline **não consegue se auto-publicar** — depois de mergeado, use
qualquer PR seguinte, ou dispare o deploy à mão pela VPS (ver *Rollback e deploy manual*).

## Secrets do GitHub

Em **Settings → Secrets and variables → Actions**:

| Secret | Para quê | Obrigatório |
| --- | --- | --- |
| `VPS_HOST` | IP ou hostname da VPS | sim |
| `VPS_USER` | usuário de deploy (`deploy`) | sim |
| `VPS_SSH_KEY` | chave privada SSH desse usuário, conteúdo inteiro | sim |
| `VPS_SSH_PORT` | porta do SSH, se não for 22 | não |
| `VPS_KNOWN_HOSTS` | chave pública do host (`ssh-keyscan -H <ip>`) | não, mas recomendado |
| `VERCEL_TOKEN` | token **de conta**, com escopo no time — não um token de projeto (ver abaixo) | sim |
| `VERCEL_ORG_ID` | está em `frontend/.vercel/project.json` após um `vercel link` | sim |
| `VERCEL_PROJECT_ID` | mesmo arquivo | sim |

Sem `VPS_KNOWN_HOSTS` o workflow cai para `ssh-keyscan`, que confia em quem quer que responda na
primeira conexão de cada execução. Ele emite um warning quando faz isso.

Não existe secret para o domínio da API: o `vps-deploy.sh` lê `API_DOMAIN_PROD` do `.env` da própria
VPS, que já é a fonte de verdade dele.

### O `VERCEL_TOKEN` não pode ser um token de projeto

A Vercel oferece dois tipos de token e só um funciona aqui. O de projeto (prefixo `vcp_`, criado
dentro de **Project Settings → Tokens**) faz o CLI falhar, e **a mensagem de erro aponta para o lugar
errado**:

```
Error: Could not retrieve Project Settings. To link your project, remove the .vercel
directory and deploy again.
```

Não é o `.vercel`. Rodando o mesmo comando com `--debug`, a sequência real aparece:

```
#1  GET /v2/user            → 404
#2  GET /v2/teams/<id>      → 403     ← este é o que mata
#3  GET /v9/.../projects    → 200
```

O CLI resolve o **time** antes do projeto, e um token de projeto não enxerga o time. Crie o token em
**Account Settings → Tokens**, com o escopo apontado para o time dono do projeto.

Se você já colou um token de projeto em algum lugar, revogue: escopo curto não é o mesmo que escopo
inofensivo.

---

## Provisionar a VPS do zero (Ubuntu)

Passo a passo, na ordem. Cada bloco é para colar no terminal.

### 1. Primeiro acesso e atualização

```bash
ssh root@<IP-DA-VPS>
apt update && apt full-upgrade -y
hostnamectl set-hostname m80
timedatectl set-timezone America/Sao_Paulo
```

O fuso **não é detalhe**: o timer do backup usa `OnCalendar=*-*-* 03:30:00`, que é hora local. Uma
VPS em UTC faria o dump às 00:30 de Brasília, no meio do expediente de emissão.

### 2. Usuário de deploy

```bash
adduser --gecos "" deploy
usermod -aG sudo deploy
rsync --archive --chown=deploy:deploy ~/.ssh /home/deploy/
```

### 3. Endurecer o SSH

`/etc/ssh/sshd_config.d/99-hardening.conf`:

```
PermitRootLogin no
PasswordAuthentication no
PubkeyAuthentication yes
KbdInteractiveAuthentication no
X11Forwarding no
MaxAuthTries 3
AllowUsers deploy
```

```bash
sudo sshd -t          # valida a sintaxe. Se falhar aqui, NÃO reinicie.
sudo systemctl restart ssh
```

> **Não feche esta sessão.** Abra **outro** terminal e teste `ssh deploy@<IP>`. Só encerre a sessão
> root depois que a nova funcionar. Ligar `PasswordAuthentication no` sem chave publicada é o jeito
> clássico de perder acesso à própria máquina e ter que ir no console web da Hostinger.

### 4. Firewall

```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow OpenSSH        # ANTES do enable — senão você se tranca para fora
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
sudo ufw status verbose
```

A 80 é obrigatória mesmo com tudo em HTTPS: é por ela que o Let's Encrypt faz o desafio HTTP-01, e é
ela que o Caddy usa para redirecionar para HTTPS.

> **O UFW não protege porta publicada por container.** O Docker escreve as próprias regras na cadeia
> `DOCKER` do iptables, **antes** das do UFW, então uma porta em `ports:` fica aberta para a internet
> mesmo com o firewall negando. O controle real é **não publicar**: no perfil prod só o `caddy-prod`
> tem `ports:`. Se um dia precisar expor algo para debug, use `127.0.0.1:8080:8080` (bind só no
> loopback) mais túnel SSH.

### 5. fail2ban e patches automáticos

```bash
sudo apt install -y fail2ban unattended-upgrades
sudo systemctl enable --now fail2ban
sudo dpkg-reconfigure --priority=low unattended-upgrades
sudo fail2ban-client status sshd
```

### 6. Swap

Os planos menores da Hostinger costumam vir sem swap. As imagens são **baixadas** do GHCR, não
compiladas aqui, então a pressão de build não existe — mas 2 GB continuam sendo seguro barato para o
Postgres:

```bash
sudo fallocate -l 2G /swapfile && sudo chmod 600 /swapfile
sudo mkswap /swapfile && sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

### 7. Docker Engine + Compose

Do repositório oficial, **não** `apt install docker.io` (versão velha, sem o plugin `compose`):

```bash
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker deploy
exit          # a lista de grupos é resolvida no login, não dinamicamente
```

Entre de novo e confirme, **antes** de seguir:

```bash
id -nG | grep -q docker && docker ps >/dev/null && echo ok
```

Sem o relogin, todo comando docker responde `permission denied while trying to connect to the Docker
API at unix:///var/run/docker.sock` — o que parece problema de instalação e é só a sessão velha
carregando os grupos antigos. Um `sudo su - deploy` de dentro da mesma sessão **não** resolve.

> Estar no grupo `docker` equivale a ser root: quem alcança o socket sobe container privilegiado e
> monta o disco do host. Aqui isso não amplia nada, porque o `deploy` já está no `sudo` — mas não
> replique o grupo para usuários que não devam ser root.

### 8. Repositório e `.env`

```bash
sudo mkdir -p /opt/m80 && sudo chown deploy:deploy /opt/m80
git clone git@github.com:murandido/m80.git /opt/m80   # deploy key read-only no GitHub
cd /opt/m80/backend
cp .env.sample .env && chmod 600 .env
nano .env      # preencher todo o bloco _PROD
```

Gere os segredos na hora — **não reaproveite os de dev**:

```bash
openssl rand -hex 32     # ENCRYPTION_KEY_PROD (64 hex = 32 bytes, exigido pelo AES-256-GCM)
openssl rand -hex 32     # CPF_INDEX_KEY_PROD (idem — HMAC do blind index de CPF)
openssl rand -base64 48  # JWT_SECRET_PROD
openssl rand -base64 24  # DB_PASSWORD_PROD e ADMIN_PASSWORD_PROD
```

Os que não existiam antes deste pipeline:

| Variável | O quê |
| --- | --- |
| `API_DOMAIN_PROD` | hostname que o Caddy serve, ex: `api.exemplo.com.br` |
| `CORS_ALLOWED_ORIGINS_PROD` | origem do painel na Vercel. Vazio libera nenhuma origem cross-site — e isso basta |
| `ADMIN_PHONE_PROD` / `ADMIN_PASSWORD_PROD` | o primeiro admin. Sem os dois a API sobe **sem usuário nenhum** |

Login no GHCR (as imagens são privadas porque o repositório é):

```bash
read -s GHCR_PAT && echo "$GHCR_PAT" | docker login ghcr.io -u murandido --password-stdin && unset GHCR_PAT
```

O `read -s` mantém o token fora do histórico do shell.

**Tem que ser um token classic**, não fine-grained — o registry não aceita os novos:

> "GitHub Packages only supports authentication using a personal access token (classic)."
> — [docs.github.com](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)

Marque **somente `read:packages`**. A VPS só precisa baixar, e o clone do repositório é feito pela
deploy key, então esse token não precisa de `repo` nem de escrita. O que se perde em relação ao
fine-grained é o escopo por repositório: um classic vale para a conta inteira. A compensação
possível é **prazo curto** — 90 dias, com lembrete no calendário, porque quando ele expira o sintoma
é um deploy falhando no `pull` e fazendo rollback sozinho.

Se um dia o `docker pull` devolver 403 num pacote privado, o motivo é o pacote herdar o acesso do
repositório: resolva em **Package settings → Manage Actions access**, não ampliando o token.

Ainda neste passo, crie o diretório de backups **já com dono `deploy`**:

```bash
sudo install -d -o deploy -g deploy -m 700 /var/backups/m-80
```

O `db-backup.sh` faz `install -d -m 700 "$BACKUP_DIR"`, mas roda como `deploy` — e `/var/backups` é
`root:root 755`, então ele não consegue criar o subdiretório sozinho. O sintoma não aparece no
primeiro deploy (que pula o dump, porque não há banco rodando ainda): aparece no **segundo**, que
morre no dump antes de puxar imagem nenhuma.

### 9. DNS — antes de subir o Caddy

- `api.<domínio>` → IP da VPS

```bash
dig +short api.<domínio>     # tem que devolver o IP da VPS antes de qualquer start
```

Subir o Caddy antes do A record resolver faz o Let's Encrypt falhar em loop e **queimar rate limit**
(5 falhas por hora, por domínio) — aí você espera, não conserta.

### 10. Primeira subida

```bash
cd /opt/m80/backend
IMAGE_TAG=latest docker compose --profile prod up -d
curl https://api.<domínio>/health     # tem que responder "API On 🚀"
```

As imagens só existem no GHCR depois do primeiro `deploy:vps`, então este passo vem **depois** de um
deploy pelo Actions — ou de um `docker build`/`push` manual.

### 11. Timer de backup

```bash
sudo cp /opt/m80/deploy/systemd/m80-backup.{service,timer} /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now m80-backup.timer
sudo systemctl start m80-backup.service      # roda uma vez agora, para provar
journalctl -u m80-backup -n 50
sudo ls -lh /var/backups/m-80
```

Detalhes de retenção e recuperação estão em [`backup.md`](backup.md).

---

## Criar o projeto na Vercel

1. **Add New → Project** → importar `murandido/m80`.
2. **Root Directory: `frontend`**. É este campo que faz a Vercel olhar só a pasta do painel: o build
   roda de dentro dela, e `frontend/pnpm-lock.yaml` + `frontend/pnpm-workspace.yaml` viram a raiz do
   workspace pnpm — que é exatamente o layout deste repo.
3. Deixe **"Include files outside the Root Directory"** *desligado*. Ligado, a Vercel manda o
   monorepo inteiro (incluindo `backend/`) para o build: mais lento, sem motivo.
4. Framework preset: **Next.js** (detectado sozinho). Install/build/output no padrão.
5. **Production Branch: `main`**.

### Variáveis de ambiente

Settings → Environment Variables, escopo **Production** (e Preview, se quiser previews funcionando):

| Variável | Valor |
| --- | --- |
| `BACKEND_URL` | `https://api.<domínio>` |

`NODE_ENV` **não** precisa ser cadastrada — a Vercel já define `production` no build, e é isso que o
`envSchema` lê. Se `BACKEND_URL` faltar, o build **falha** em `next.config.ts`
(`envSchema.parse(process.env)`), que é o comportamento desejado: melhor quebrar no build do que
subir um painel que não fala com nada.

### Domínio

Settings → Domains → adicionar `painel.<domínio>` e criar o registro que a Vercel indicar. **Essa
origem exata é o valor de `CORS_ALLOWED_ORIGINS_PROD`** no `.env` da VPS.

### O deploy automático já vem desligado

`frontend/vercel.json` traz `{"git": {"deploymentEnabled": {"main": false}}}`. Sem isso a Vercel
publicaria a cada push na main e a label não controlaria nada.

### Um limite a conhecer (não é bug)

`frontend/src/app/api/events/route.ts` declara `maxDuration = 300`. No plano **Hobby** a Vercel corta
em 60s, então o SSE reconecta a cada minuto em vez de a cada cinco. O `useSSE` já delega reconexão ao
`EventSource` nativo, então nada quebra — mas são ~60 invocações por hora, por aba aberta.

---

## Os workflows, e o que custou caro descobrir

Cada item aqui derrubou pelo menos um deploy. Estão registrados porque nenhum deles é dedutível
lendo a documentação da ferramenta.

### O CLI da Vercel roda da raiz do repositório

O workflow **não** tem `working-directory: frontend`, por mais que pareça certo. O projeto na Vercel
já tem **Root Directory: `frontend`**, e o CLI aplica esse caminho ele mesmo — rodando de dentro da
pasta, ele procura `frontend/frontend/package.json` e falha com um `ENOENT` seco. O comentário está
no `.github/workflows/deploy-vercel.yml`; não o remova junto com o `working-directory` na próxima vez
que ele parecer sobrando.

### O runner precisa de `pnpm` instalado

`vercel build` faz a instalação de dependências sozinho e chama `pnpm`, que o runner não tem:
`setup-node` traz só npm, e o ambiente de build da própria Vercel já vem com pnpm — então esse buraco
só existe fora de lá. O erro é `spawn pnpm ENOENT`.

A versão é **fixada em 10**, que é o que a Vercel escolhe para este projeto (ela diz isso no log de
build: um lockfile 9.0 é ambíguo entre pnpm 9 e 10 e ela desempata pela data de criação do projeto).
As duas têm que concordar, ou o deploy resolve dependências com um resolver diferente de todo preview.

### Não há cache de layer no build das imagens

`cache-from`/`cache-to: type=gha` falhava **todo** build com `failed to reserve cache` — e falhava
*depois* de a imagem já ter sido construída e enviada, então lia como falha de build. Nem o default de
permissão do repositório nem conceder `actions: write` explicaram: a cota estava pela metade e
nenhuma entrada era do buildx. Dois deploys foram gastos nisso.

Cache é otimização e essa estava bloqueando produção, então foi removida em vez de chutada uma
terceira vez.

**A causa apareceu depois, no job vizinho.** O deploy de 09/08 trouxe uma anotação que o buildx nunca
emitiu:

```
Cache reservation failed: cache write denied: token has no writable scopes
```

O padrão de permissão do repositório é `read`, e o serviço de cache do Actions recusa token sem
escopo de escrita.

O detalhe que escondeu isso por duas tentativas: **o mesmo `ci.yml` cacheava normalmente em todo pull
request** e só falhava quando o deploy o chamava. Um workflow reusável nunca tem mais permissão que o
job que o chama, e o job `ci` do `deploy-vps.yml` não declarava bloco nenhum — então caía no padrão
do repositório, e as permissões que o `ci.yml` declara para si mesmo eram rebaixadas em silêncio. A
correção é o `permissions:` **no job chamador**.

**O cache de layer do buildx continua desligado.** O job `build` tem o mesmo defeito (concede
`contents` e `packages`, então `actions` fica em `none`), mas essa hipótese já pareceu boa antes e o
lugar de testá-la não é o caminho de deploy de produção.

### O SSH insiste por dez minutos antes de desistir

Alguns deploys morreram em `connect to host: Connection timed out` sem nunca chegar à VPS — os dois
da montagem em 08/08 e um em 27/08.

O de 27/08 foi investigado com a caixa na mão, e **todos os suspeitos caíram**:

| Suspeito | Como foi descartado |
|---|---|
| `sshd` fora do ar | `active`, escutando em `0.0.0.0:22` e `[::]:22`; 119 dias de uptime, sem reboot |
| Firewall do painel Hostinger | Regra `Accept TCP 22 · Fonte Any` acima do `Drop` |
| UFW | Regra não muda sozinha, e o deploy de 25/08 passou pelo mesmo caminho |
| fail2ban | Jail é a `defaults-debian.conf` pura, e o `/var/log/fail2ban.log` **não foi escrito no dia** — o `mtime` parou em 26/08 |
| Secret `VPS_HOST` apontando para IP velho | Os `VPS_*` não são tocados desde 07/08, e uma host key de 09/06 ainda casa: mesma caixa, mesmo endereço |

Sobrou a **rota** entre o runner e a VPS. Não há o que consertar do nosso lado disso, então o que o
workflow faz é sobreviver a ela.

Uma armadilha ao investigar isto: **muita rede corporativa e de universidade bloqueia a porta 22 de
saída**. Testar `ssh` de uma dessas mede o bloqueio local, não a VPS, e o sintoma é idêntico. Antes
de concluir qualquer coisa a partir da sua máquina, confirme com um alvo conhecido:

```bash
timeout 10 bash -c 'cat < /dev/null > /dev/tcp/github.com/22' && echo aberto || echo bloqueado
```

Se o `github.com:22` também estourar, o problema é a sua rede — use VPN e refaça o teste.

O que existe no job é uma **sonda de conexão** repetida a cada 30s por até **dez minutos**; só o
`true` remoto é repetido, nunca o deploy — repetir o deploy refaria o dump e recriaria todo
container. A janela é generosa de propósito e não custa nada num deploy saudável: a primeira sonda
responde e o laço sai. As cinco tentativas anteriores cobriam 2min30, curto demais para atravessar
um soluço de rota.

Se a janela inteira falhar, o job imprime duas coisas antes de abortar:

- o **IP de saída do runner**, que muda a cada execução e distingue "um endereço bloqueado" de "uma
  faixa inteira";
- se o **443 responde** desse mesmo runner. Caddy atende no 443 sempre que a VPS está viva, então
  443 sim e 22 não é filtragem específica de SSH (olhe firewall), e nenhum dos dois é a rota caída
  (não há o que fazer). É a pergunta que as falhas anteriores deixaram em aberto.

---

## O que um deploy faz

`deploy/vps-deploy.sh <sha>`, na ordem:

1. **Dump do banco.** A API roda as migrations sozinha no boot, e migration é a única parte de um
   deploy que rollback não desfaz.
2. `docker compose --profile prod pull`.
3. `up -d` — o Compose recria só o que mudou de imagem.
4. **Health check** em `https://$API_DOMAIN/health`, com retry por 90s.
5. **Rollback automático** para a tag anterior se o health nunca passar. As duas tags ficam em
   `deploy/.deploy-state` (não versionado).

O `git fetch`/`checkout` acontece **fora** do script, no workflow: bash lê o arquivo do script aos
pedaços, e um script que reescreve o próprio arquivo no meio da execução se comporta de formas que
ninguém quer depurar de madrugada.

## Rollback e deploy manual

```bash
cd /opt/m80
git fetch origin main
git checkout --detach <sha-anterior>
deploy/vps-deploy.sh <sha-anterior>
```

**O SHA é o completo, de 40 caracteres.** As imagens são publicadas com a tag que o job `resolve`
produz, que é o `git rev-parse HEAD` inteiro; passar o SHA curto que o `git log` mostra faz o `pull`
morrer em `manifest unknown` e o script aborta sem tocar em nada. Já custou uma tentativa de deploy.

Toda tag antiga continua no GHCR, então qualquer SHA já publicado serve.

**O rollback automático troca só as imagens.** Se o deploy que quebrou também mexeu no
`docker-compose.yml` ou no `Caddyfile`, faça o `git checkout` do commit anterior antes de rodar o
script — é o comando acima.

## Verificações que valem fazer

- **Quebre um deploy de propósito** uma vez e confirme que o script volta sozinho para o SHA
  anterior. Rollback nunca exercitado não é rollback — mesma regra do backup.
- `curl https://api.<domínio>/swagger/index.html` → **404** em produção.
- `nmap <ip>` mostrando só 22/80/443.
- No painel, DevTools → Application → Cookies: `auth_token` com `Secure` e `HttpOnly`.
- Push na main **sem label**: nada é publicado dos dois lados.

## Fora de escopo, e vale saber

Nada disto é pré-requisito para ir ao ar, mas nenhum está feito:

- rotação de `JWT_SECRET` / `ENCRYPTION_KEY` / `CPF_INDEX_KEY` — e note que as duas últimas
  não são descartáveis: `ENCRYPTION_KEY` decifra senha de conta e documento de passageiro, e
  `CPF_INDEX_KEY` é o que faz o beneficiário já gravado ser reconhecido pelo CPF. Trocar
  qualquer uma sem migrar o dado existente não dá erro — passa a errar em silêncio;
- cópia off-site do backup — hoje o dump vive no mesmo disco do banco (ver `backup.md`);
- 2FA no painel e política de senha no cadastro de usuário.
