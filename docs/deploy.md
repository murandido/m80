# Deploy

O M-80 inteiro — **API, bot, promo-service, pdf-service, Postgres e o painel Next.js** — roda em
Docker numa máquina Oracle Ampere A1 (ARM64), atrás do Caddy, que serve **um único domínio**. É
publicado por **label no GitHub**, nunca por push ou merge.

Este documento é o runbook: como publicar, como provisionar do zero, e o que fazer quando dá errado.

## Merge não é publicação

`main` nem sempre é versão final. Por isso mergear e publicar são dois atos separados:

1. Mergeie o PR normalmente (squash merge, como sempre).
2. Quando aquilo tiver que ir ao ar, aplique a label no PR **já mergeado**:

| Label | Publica |
| --- | --- |
| `deploy:vps` | a stack inteira, painel incluso (`.github/workflows/deploy-vps.yml`) |

É uma label só desde a migração para a Oracle: painel e API são publicados juntos, no mesmo SHA.
Antes eram duas independentes, e o preço disso era ficarem fora de sincronia por dias.

**`deploy:ca` não é desta máquina.** Ela publica a branch `ca-mode` no ambiente CA's Mode, uma VPS
separada com dados anonimizados. O workflow e o runbook (`docs/ca-mode.md`) vivem naquela branch.
Cada label só dispara com a própria branch como base — `deploy:vps` exige um PR para `main` — então
trocar uma pela outra não publica nada no lugar errado: o run aparece como *skipped*.

**Para republicar a mesma versão**, tire e recoloque a label: o gatilho é o evento `labeled`, então
uma label que já está lá não dispara nada.

**O que vai ao ar é a `main`, não o PR.** Como o PR já foi mergeado, o primeiro job resolve o SHA da
`main` e todos os outros se fixam nele. Se alguém der push na main no meio do deploy, o que subiu
continua sendo um commit só, inteiro.

**Na primeira vez tem uma pegadinha:** o workflow precisa já estar na `main` para uma label disparar.
O próprio PR que introduz este pipeline **não consegue se auto-publicar** — depois de mergeado, use
qualquer PR seguinte, ou dispare o deploy à mão na própria máquina (ver *Rollback e deploy manual*).

## Secrets do GitHub

Em **Settings → Secrets and variables → Actions**:

| Secret | Para quê | Obrigatório |
| --- | --- | --- |
| `VPS_HOST` | IP ou hostname da máquina de produção | sim |
| `VPS_USER` | usuário de deploy (`deploy`) | sim |
| `VPS_SSH_KEY` | chave privada SSH desse usuário, conteúdo inteiro | sim |
| `VPS_SSH_PORT` | porta do SSH, se não for 22 | não |
| `VPS_KNOWN_HOSTS` | chave pública do host (`ssh-keyscan -H <host>`) | não, mas recomendado |

Sem `VPS_KNOWN_HOSTS` o workflow cai para `ssh-keyscan`, que confia em quem quer que responda na
primeira conexão de cada execução. Ele emite um warning quando faz isso.

Não existe secret para o domínio da API: o `vps-deploy.sh` lê `API_DOMAIN_PROD` do `.env` da própria
máquina, que já é a fonte de verdade dele.


---

## Provisionar a máquina do zero (Oracle Cloud, Ubuntu ARM64)

Passo a passo, na ordem. Cada bloco é para colar no terminal.

> Este runbook descrevia uma VPS x86 da Hostinger até 29/08/2026. A produção passou a rodar
> numa **Oracle Ampere A1 (aarch64)**, e as diferenças não são cosméticas: são **dois**
> firewalls em vez de um, o IP público **não** vem por padrão, e o shape só é obtido em conta
> paga. O que valia para qualquer Ubuntu — fuso, usuário de deploy, grupo `docker`, GHCR —
> continua igual.

### 0. Criar a instância

Shape **VM.Standard.A1.Flex**, **2 OCPU / 12 GB**, imagem **Canonical Ubuntu 24.04 aarch64**,
boot volume até 200 GB. Confira o selo **Always Free-eligible** no shape: é ele que garante que a
máquina sobreviva ao fim do trial.

Três armadilhas do assistente de criação, todas já pagas:

- **"Out of capacity for shape VM.Standard.A1.Flex".** Não é erro de configuração — é falta de
  host. São Paulo tem **um único AD**, então a sugestão do próprio erro ("tente outro
  availability domain") não se aplica. O que destravou foi **subir a conta para Pay As You Go**:
  na primeira tentativa seguinte a instância criou. Recurso Always Free só existe na home region,
  e a home region não muda depois que a conta é criada.
- **A lista de imagens é filtrada pelo shape.** Com o shape x86 padrão, nenhuma imagem aarch64
  aparece e a busca por "aarch64" não devolve nada. **Escolha o shape primeiro**, depois a imagem.
- **O IP público não vem.** Criando VCN e subnet pelo próprio assistente, o toggle
  "Automatically assign public IPv4 address" fica **desabilitado**, e o Review mostra
  `Public IPv4 address: No`. O internet gateway *é* criado; só o IP que não. Resolva depois em
  **Instância → VNIC → IP administration → Edit → Reserved public IP → Create new**. Prefira
  **reservado**: o efêmero some num stop/start e o DNS passa a apontar para o nada.

As chaves SSH ficam no fim da etapa **Networking**, abaixo de "Advanced options" — não numa etapa
própria. Cole sua chave pública em vez de deixar a Oracle gerar o par: a privada só pode ser
baixada uma vez, e perdê-la significa recriar a instância.

O usuário inicial é `ubuntu`, não `root`.

### 1. Primeiro acesso e fuso

```bash
ssh ubuntu@<IP>
sudo apt update && sudo apt full-upgrade -y
sudo timedatectl set-timezone America/Sao_Paulo
```

O fuso **não é detalhe**: o timer do backup usa `OnCalendar=*-*-* 03:30:00`, que é hora local. Uma
máquina em UTC faria o dump às 00:30 de Brasília, no meio do expediente de emissão.

O hostname já vem como o nome da instância e casa com o registro DNS interno da OCI — não troque.

### 2. Usuário de deploy

```bash
sudo adduser --disabled-password --gecos "" deploy
sudo usermod -aG sudo deploy
sudo rsync --archive --chown=deploy:deploy /home/ubuntu/.ssh /home/deploy/
echo "deploy ALL=(ALL) NOPASSWD:ALL" | sudo tee /etc/sudoers.d/90-deploy
sudo chmod 0440 /etc/sudoers.d/90-deploy && sudo visudo -c -f /etc/sudoers.d/90-deploy
```

O `--disabled-password` é o que se quer (acesso só por chave), mas ele deixa o `sudo` pedindo uma
senha que não existe — o usuário fica no grupo `sudo` sem conseguir elevar. Daí o drop-in
`NOPASSWD`, que é exatamente como a Oracle já configura o `ubuntu`. Isso não amplia poder: quem
está no grupo `docker` já alcança root pelo socket.

### 3. Endurecer o SSH

`/etc/ssh/sshd_config.d/99-hardening.conf`:

```
PermitRootLogin no
PasswordAuthentication no
PubkeyAuthentication yes
KbdInteractiveAuthentication no
X11Forwarding no
MaxAuthTries 3
AllowUsers deploy ubuntu
```

```bash
sudo sshd -t          # valida a sintaxe. Se falhar aqui, NÃO reinicie.
sudo systemctl restart ssh
```

> **Não feche esta sessão.** Abra **outro** terminal e teste `ssh deploy@<host>`. Só encerre a
> atual depois que a nova funcionar.

`ubuntu` fica no `AllowUsers` de propósito: na OCI ele é a conta break-glass da plataforma, é
só-chave igual ao `deploy`, e sem ela a recuperação depende do console serial. Não amplia
superfície — mesma chave, senha desabilitada. Aperte para `deploy` sozinho quando quiser.

### 4. Firewall — são dois, e o UFW não entra aqui

**Não instale UFW nesta máquina.** A imagem da Oracle já vem com iptables gerenciado por
`netfilter-persistent`, e empilhar um segundo gerenciador arrisca reescrever a cadeia e derrubar o
SSH. As duas camadas que existem:

**(a) Security List da VCN** — `Networking → VCN → Security Lists → Default → Add Ingress Rules`.
Só a 22 vem aberta. Adicione, com Source CIDR `0.0.0.0/0`, TCP:

| Porta | Por quê |
| --- | --- |
| `80` | desafio HTTP-01 do Let's Encrypt e o redirect do Caddy para HTTPS |
| `443` | o painel |

**(b) iptables local** — a imagem rejeita tudo que não seja SSH. A regra `REJECT` é a última da
cadeia `INPUT`, então as novas têm que entrar **antes** dela:

```bash
sudo iptables -L INPUT -n --line-numbers      # descubra a posição do REJECT
sudo iptables -I INPUT 5 -m state --state NEW -p tcp --dport 80  -j ACCEPT
sudo iptables -I INPUT 5 -m state --state NEW -p tcp --dport 443 -j ACCEPT
sudo netfilter-persistent save
```

Abrir só uma das duas camadas dá um sintoma idêntico ao de DNS errado — Caddy falhando em emitir
certificado, health check do deploy expirando — e é por isso que confunde. Prove as duas de uma vez
com um listener temporário:

```bash
# na máquina:
sudo timeout 30 python3 -m http.server 80
# de fora:
curl -s -o /dev/null -w '%{http_code}\n' http://<host>/
```

> **O firewall não protege porta publicada por container.** O Docker escreve as próprias regras
> antes das do host, então uma porta em `ports:` fica aberta para a internet de qualquer jeito. O
> controle real é **não publicar**: no perfil prod só o `caddy-prod` tem `ports:`. Para debug, use
> `127.0.0.1:8080:8080` mais túnel SSH.

### 5. fail2ban e patches automáticos

```bash
sudo apt install -y fail2ban unattended-upgrades
sudo systemctl enable --now fail2ban
sudo dpkg-reconfigure --priority=low unattended-upgrades
sudo fail2ban-client status sshd
```

### 6. Swap

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

### 8. Repositório e `.env`

```bash
sudo mkdir -p /opt/m80 && sudo chown deploy:deploy /opt/m80
git clone git@github.com:murandido/m80.git /opt/m80   # deploy key read-only no GitHub
```

A deploy key é por máquina: gere o par **na própria máquina**, como `deploy`, e cadastre só a
pública em **Settings → Deploy keys**, sem acesso de escrita.

```bash
ssh-keygen -t ed25519 -N "" -C "m80-prod-oracle-deploy-key" -f ~/.ssh/id_ed25519_m80
```

#### O `.env`: instalação nova ≠ migração

Numa **instalação nova**, gere os segredos na hora — não reaproveite os de dev:

```bash
openssl rand -hex 32     # ENCRYPTION_KEY_PROD (64 hex = 32 bytes, exigido pelo AES-256-GCM)
openssl rand -hex 32     # CPF_INDEX_KEY_PROD (idem — HMAC do blind index de CPF)
openssl rand -base64 48  # JWT_SECRET_PROD
openssl rand -base64 24  # DB_PASSWORD_PROD e ADMIN_PASSWORD_PROD
```

Numa **migração para outra máquina, isso está errado e destrói dado.**
`ENCRYPTION_KEY_PROD` e `CPF_INDEX_KEY_PROD` têm que ser **idênticas** às da máquina antiga: elas
são o que dá sentido aos documentos de passageiro já gravados e ao blind index de CPF. Chave nova
não "quebra" nada visivelmente — o banco continua cheio, e o conteúdo vira ilegível. Não aparece no
boot nem nos testes; aparece quando alguém abre um bilhete antigo, e a essa altura não há volta.

Copie o arquivo inteiro, direto de uma máquina para a outra, sem escalar em disco:

```bash
ssh deploy@<host-antigo> 'cat /opt/m80/backend/.env' \
  | ssh deploy@<host-novo> 'cat > /opt/m80/backend/.env && chmod 600 /opt/m80/backend/.env'
```

Depois ajuste só o que muda de endereço, e **confira que nenhuma chave `_PROD` ficou faltando** —
uma variável ausente não é erro, é comportamento silencioso (ver o `postCallback` do bot, que vira
no-op quando a URL está vazia):

```bash
cd /opt/m80/backend
grep -E '^[A-Z_]+_PROD=' .env.sample | sed 's/=.*//' | sort > /tmp/req
grep -E '^[A-Z_]+_PROD=' .env        | sed 's/=.*//' | sort > /tmp/got
comm -23 /tmp/req /tmp/got     # o que falta no .env
```

As variáveis que não existiam antes deste pipeline:

| Variável | O quê |
| --- | --- |
| `API_DOMAIN_PROD` | o **único** hostname que o Caddy serve. Desde a migração ele serve o painel e roteia só `/health` para a API |
| `CORS_ALLOWED_ORIGINS_PROD` | vazio, em definitivo. Com painel e API na mesma caixa a API não é publicada, então não existe origem cross-site para liberar |
| `ADMIN_PHONE_PROD` / `ADMIN_PASSWORD_PROD` | o primeiro admin. Sem os dois a API sobe **sem usuário nenhum** |

Login no GHCR (as imagens são privadas porque o repositório é), **como `deploy`**, que é quem o
`vps-deploy.sh` usa:

```bash
read -rs GHCR_PAT && echo "$GHCR_PAT" | docker login ghcr.io -u murandido --password-stdin && unset GHCR_PAT
```

O `read -rs` e o `--password-stdin` mantêm o token fora do histórico do shell e de `ps`.

**Tem que ser um token classic**, não fine-grained — o registry não aceita os novos:

> "GitHub Packages only supports authentication using a personal access token (classic)."
> — [docs.github.com](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)

Marque **somente `read:packages`**. A máquina só precisa baixar, e o clone é feito pela deploy key,
então esse token não precisa de `repo` nem de escrita. O que se perde em relação ao fine-grained é o
escopo por repositório: um classic vale para a conta inteira. Sobre validade, escolha consciente: um
prazo curto obriga rotação, mas quando ele expira o sintoma é um deploy falhando no `pull` e fazendo
rollback sozinho — se optar por prazo, coloque lembrete no calendário.

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

- `<domínio>` → **IP reservado** da instância

```bash
dig +short <domínio>     # tem que devolver o IP antes de qualquer start
```

Subir o Caddy antes do A record resolver faz o Let's Encrypt falhar em loop e **queimar rate limit**
(5 falhas por hora, por domínio) — aí você espera, não conserta.

### 10. Secrets do deploy

O workflow entra por SSH como `deploy`. Autorize a chave do CI na máquina nova e aponte os secrets
para ela — assim `VPS_SSH_KEY` não precisa ser tocado:

```bash
# na máquina, como deploy:
echo '<conteúdo de m80_ci.pub>' >> ~/.ssh/authorized_keys

# na sua máquina:
ssh-keyscan -H <domínio> | gh secret set VPS_KNOWN_HOSTS --repo murandido/m80
printf '%s' '<domínio>' | gh secret set VPS_HOST --repo murandido/m80
printf '%s' 'deploy'    | gh secret set VPS_USER --repo murandido/m80
```

Confirme simulando o que o workflow faz, com `StrictHostKeyChecking=yes`:

```bash
ssh -i ~/.ssh/m80_ci -o IdentitiesOnly=yes deploy@<domínio> 'cd /opt/m80 && git fetch origin main'
```

### 11. Primeira subida

```bash
cd /opt/m80/backend
IMAGE_TAG=latest docker compose --profile prod up -d
curl https://<domínio>/health     # tem que responder "API On 🚀"
```

As imagens só existem no GHCR depois do primeiro `deploy:vps`, então este passo vem **depois** de um
deploy pelo Actions. **E elas precisam ser da arquitetura certa:** imagem amd64 não sobe em ARM.
Confira antes de perder tempo depurando:

```bash
docker buildx imagetools inspect ghcr.io/murandido/m80-api:<sha>
```

### 12. Timer de backup

```bash
sudo cp /opt/m80/deploy/systemd/m80-backup.{service,timer} /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now m80-backup.timer
sudo systemctl start m80-backup.service      # roda uma vez agora, para provar
journalctl -u m80-backup -n 50
sudo ls -lh /var/backups/m-80
```

Detalhes de retenção e recuperação estão em [`backup.md`](backup.md).

### 13. Conta paga: ponha um alerta de custo

Pay As You Go **não tem teto rígido de gasto** — nenhum provedor grande tem. O que protege é a
franquia Always Free continuar sendo abatida (2 OCPU × 744 h = 1.488 das 1.500 OCPU-horas
gratuitas; 12 GB × 744 h = 8.928 das 9.000 GB-horas) e um alerta seu.

Em **Billing & Cost Management → Budgets**, crie um budget de **US$1** com regra de alerta em
**Actual Spend, 1%** — ou seja, dispara em **US$0,01**. Com margem de 12 OCPU-horas no mês, esse
alerta chega antes de virar dinheiro relevante.

---


---

## Os workflows, e o que custou caro descobrir

Cada item aqui derrubou pelo menos um deploy. Estão registrados porque nenhum deles é dedutível
lendo a documentação da ferramenta.

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

Alguns deploys morreram em `connect to host: Connection timed out` sem nunca chegar à máquina — os
dois da montagem em 08/08 e um em 27/08. **Isso é histórico da VPS da Hostinger**, aposentada em
29/08/2026; a investigação fica registrada porque a mitigação no workflow continua lá e porque o
método de descarte serve para o próximo caso.

O de 27/08 foi investigado com a caixa na mão, e **todos os suspeitos caíram**:

| Suspeito | Como foi descartado |
|---|---|
| `sshd` fora do ar | `active`, escutando em `0.0.0.0:22` e `[::]:22`; 119 dias de uptime, sem reboot |
| Firewall do painel Hostinger | Regra `Accept TCP 22 · Fonte Any` acima do `Drop` |
| UFW | Regra não muda sozinha, e o deploy de 25/08 passou pelo mesmo caminho |
| fail2ban | Jail é a `defaults-debian.conf` pura, e o `/var/log/fail2ban.log` **não foi escrito no dia** — o `mtime` parou em 26/08 |
| Secret `VPS_HOST` apontando para IP velho | Os `VPS_*` não são tocados desde 07/08, e uma host key de 09/06 ainda casa: mesma caixa, mesmo endereço |

Sobrou a **rota** entre o runner e a máquina. Não há o que consertar do nosso lado disso, então
o que o
workflow faz é sobreviver a ela.

Uma armadilha ao investigar isto: **muita rede corporativa e de universidade bloqueia a porta 22 de
saída**. Testar `ssh` de uma dessas mede o bloqueio local, não a máquina, e o sintoma é idêntico.
Antes
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
- se o **443 responde** desse mesmo runner. Caddy atende no 443 sempre que a máquina está viva, então
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

### O promo é recriado, e um cadastro em andamento morre junto

`mem_limit` não é aplicável a quente, então qualquer deploy que mexa nele **recria** o
`promo-service-prod` em vez de deixá-lo de pé. Um cadastro de promoção rodando naquele instante é
perdido. Não é grave — a idempotência por `url_key` faz o mesmo link recadastrar só quem ficou de
fora — mas é motivo para não fazer deploy no meio de uma campanha aberta.

## O teto de memória do promo

O `promo-service-prod` é o **único** container com limite (`mem_limit: 1500m`, `memswap_limit` igual
— sem swap — e `shm_size: 256mb` saindo *de dentro* desse teto). O raciocínio está no
`backend/CLAUDE.md`; aqui fica só o que se faz quando ele aparece.

Como se percebe:

```bash
docker inspect promo-service-prod --format '{{.State.OOMKilled}} {{.RestartCount}}'
docker compose --profile prod ps          # o promo deve aparecer (healthy)
docker stats --no-stream promo-service-prod
```

`OOMKilled: true` com `RestartCount` subindo é o teto sendo batido de verdade. O container volta
sozinho — o serviço encerra quando o browser some e o `restart: always` faz o resto — então **nada
some em silêncio**: o grupo recebe ⚠️ no cadastro que morreu.

Se for campanha legítima batendo no teto, suba o `mem_limit` — a caixa tem folga de sobra: o teto
foi dimensionado numa VPS de 3.8 GB e a Oracle tem 12 GB, com ~10 GB livres. Baixar o
`PROMO_MAX_CPFS` também alivia, porque os screenshots ficam em base64 no heap até a resposta sair,
mas é tratar sintoma.

Um container `(unhealthy)` **sem** `OOMKilled` é outra coisa: o Chrome morreu por conta própria.
`docker logs promo-service-prod` tem o motivo.

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
