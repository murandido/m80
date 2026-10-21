# M-80 Workspace - Diretrizes Globais

## O Projeto
- Monorepo poliglota: `/frontend` (Next.js, servido pelo Caddy junto com a API), `/backend` (Go), um microserviço Python em `/backend/pdf-service` (Flask + WeasyPrint, geração de PDF, uso interno — ver `backend/pdf-service/CLAUDE.md`) e `/extension` (extensão de navegador para a equipe — ver *Extensão de Navegador*).
- Você está atuando como Engenheiro Full-Stack Sênior. Priorize soluções pragmáticas e evite over-engineering — implemente o que a tarefa pede, sem abstrações ou generalizações especulativas para necessidades futuras hipotéticas.

## ⚠️ O M-80 está EM PRODUÇÃO (desde 08/08/2026)

Não é mais um projeto pré-lançamento. A stack inteira roda numa máquina Oracle Ampere A1 atrás do Caddy, e há **gente de verdade emitindo bilhete e dinheiro de verdade registrado no banco**. Isso muda o padrão de trabalho, não só o tom:

- **Toda mudança passa a ter raio de alcance.** Antes de propor ou aplicar qualquer coisa, responda para si mesmo: o que quebra se isto estiver errado, e como se percebe? Se a resposta for "ninguém percebe até fechar o mês", a mudança precisa de mais cuidado, não de mais velocidade.
- **Mudança de schema é o caso mais sério.** Migration roda sozinha no boot da API e é a única parte de um deploy que o rollback **não** desfaz — o rollback troca imagem, não schema. Migration destrutiva (`DROP`, `ALTER ... TYPE`, `NOT NULL` em coluna existente, renomear coluna) exige: `make db-backup` antes, um caminho `down` que realmente funcione, e conversa com o usuário antes de escrever o arquivo.
- **Dado em produção não volta.** `DELETE` sem soft delete, seed reconciliador tocando coluna nova, correção "rápida" direto no banco — nada disso tem desfazer. A receita de recuperação parcial está em `docs/backup.md` e depende de haver dump; o dump é diário, então o pior caso é perder um dia.
- **Nunca rode nada à mão na máquina de produção ou no banco sem pedir.** Inclui `docker compose`, `psql`, `docker exec`. O caminho normal é branch → PR → merge → label.
- **API e painel sobem juntos, mas o browser não.** Desde 29/08/2026 as duas imagens são publicadas pela mesma label, então não existe mais a janela de dias em que um lado ficava à frente do outro. O que sobrou é menor e continua real: quem está com o painel aberto segue rodando o bundle antigo até recarregar a aba. Mudança que quebra contrato (renomear campo de resposta, mudar status HTTP, exigir campo novo no request) ainda merece o passo intermediário que aceita as duas formas — só que agora por minutos, não por dias.
- **Quem está exposto é o painel, não a API.** O Go deixou de ter endereço público: só o painel e o `/health` atravessam o Caddy. Isso encolheu a superfície, mas não zerou o cuidado — todo request de usuário ainda chega ao Go, agora vindo do container do Next em vez da internet, o que muda o que `middleware.RealIP` e o rate limit por IP significam. Ao mexer em middleware, auth ou env, leia `backend/internal/api/CLAUDE.md` antes.
- **Sugerir ≠ aplicar.** Diante de algo arriscado, apresente o risco e a alternativa e espere. "Já que estou aqui" é como se derruba produção.

O que **não** muda: continua valendo evitar over-engineering. Cuidado é pensar antes, não construir abstração defensiva.

## Documentação de Produto (Notion)
Duas páginas privadas no Notion são a fonte de verdade para **intenção**: o "porquê" das decisões, o schema completo, os formatos de mensagem do bot e o backlog. Este repositório é público, então os links ficam em `backend/carriers/CLAUDE.md` (submódulo privado).

Quando consultar:
- Antes de propor mudança de arquitetura ou de schema — a decisão pode já ter sido tomada e descartada, com motivo registrado.
- Ao mexer no bot ou em emissões — o formato das 5 linhas da EMISSAO e as regras de resolução de apelido estão documentados lá por extenso.
- Quando faltar o "porquê" de algo que o código não explica.

Como tratar o conteúdo:
- **O código é a fonte de verdade sobre o que existe; o Notion, sobre o que se decidiu.** A seção *Estado da Implementação* e os marcadores 🔴 (não construído) / 🟡 (revisado) mapeiam um no outro, mas podem envelhecer — confirme no repositório antes de afirmar que algo existe.
- Se encontrar divergência real entre doc e código, **avise o usuário**; não altere o Notion por conta própria.

## Infraestrutura e Comandos
- **NUNCA** sugira ferramentas de monorepo JS (Lerna, Nx, Turborepo).
- O projeto é orquestrado via `Makefile` na raiz.
- `make init` — baixa os submódulos privados (`backend/carriers` → `murandido/m80-carriers`, `frontend/src/carriers` → `murandido/m80-panel-carriers`, `extension` → `murandido/m80-extension`). Sem eles o backend não compila. **Todo código que conversa com sistema de terceiro (cias, Traveos, páginas de promoção) vive em `backend/carriers` (ou, no painel, em `frontend/src/carriers`), nunca em `backend/internal` ou `frontend/src/lib`**: o repo principal é público. Ao mexer no submódulo, commite lá dentro primeiro e depois o ponteiro aqui.
- `make dev` — sobe o Docker do Go + dev server do Next.
- `make back-down` — derruba o Docker do Go (Next continua rodando).
- `make back-logs` — stream de logs do backend.
- `make clean` — limpa caches de build (use ao trocar de branch ou após mudanças de dependência).

## Deploy
- **Um domínio só, e a API não é publicada.** Desde 29/08/2026 painel e API rodam na mesma máquina: o Caddy serve um único hostname, manda tudo para o painel e roteia apenas `/health` para a API. O Go só é alcançável pela rede interna do Compose. Por isso `API_DOMAIN_PROD` é o único lugar onde o domínio aparece, e `CORS_ALLOWED_ORIGINS_PROD` fica vazio em definitivo — não é "seguro porque ninguém chama", é que ninguém *consegue*. O domínio é `emioitenta.com.br` (zona DNS no registro.br desde 26/09/2026; substituiu o DuckDNS provisório). Mesmo assim, não o escreva literal em código: ele vem de `API_DOMAIN_PROD`, e trocá-lo tem que continuar sendo só editar o `.env`.
- **Merge não é publicação.** A `main` nem sempre é versão final, então nada vai ao ar por push ou merge. Deploy é disparado por **label num PR já mergeado**: `deploy:vps` publica a stack inteira — API, bot, promo, pdf-service **e painel**. Runbook completo em `docs/deploy.md`.
- **Existe um segundo ambiente, e ele não é produção.** A branch longeva `ca-mode` roda numa VPS Oracle separada, para o designer usar telas que ainda não foram decididas, com um dump de produção anonimizado. Publica com a label `deploy:ca` (nunca `deploy:vps`), tem secrets `CA_VPS_*` e tag de imagem `ca-latest` em vez de `latest`. Reusa o profile `prod` do Compose de propósito — o que separa os dois ambientes é o `.env` de cada máquina. O runbook (`docs/ca-mode.md`), o workflow (`deploy-ca.yml`) e o anonimizador (`backend/cmd/anonymize`) vivem **só na branch `ca-mode`**. Duas consequências para quem mexe na `main`: **antes de criar uma migration, confira o último número nas duas branches** (número duplicado só aparece no rebase ou no deploy), e **nunca publique `latest` a partir da `ca-mode`** — é a tag que a produção resolve.
- A máquina **baixa** as imagens do GHCR (`ghcr.io/murandido/m80-{api,bot,promo,pdf-service,frontend}`), não compila nada. Elas são **arm64**: a produção roda num Oracle Ampere A1, e imagem amd64 não sobe lá. O perfil `prod` do Compose usa `image:`, não `build:` — não reintroduza `build:` lá.
- Em produção a API roda atrás do **Caddy** (`deploy/caddy/Caddyfile`), que resolve TLS sozinho. Só o `caddy-prod` publica portas: o Docker escreve as próprias regras de iptables antes das do host, então uma porta em `ports:` fica aberta para a internet de qualquer jeito — **não publicar é o controle real**.
- `deploy/vps-deploy.sh` faz dump → pull → up → health check → rollback automático para a tag anterior. **Rode `make db-backup` antes de qualquer coisa arriscada feita à mão** — o script já faz isso sozinho.
- Não mexa em `.github/workflows/` sem pedido expresso (ver *Fluxo de Trabalho*).
- **Changelog para a equipe:** quando o usuário pedir o aviso das novidades de um deploy, use a skill `changelog` (`.claude/skills/changelog/SKILL.md`). O texto vai para o grupo de WhatsApp dos funcionários, em linguagem simples, só com o que a equipe vê (novidades, correções, coisas retiradas). Quem cola no grupo é o usuário.

## Extensão de Navegador
- `/extension` é um submódulo privado (`murandido/m80-extension`). As regras dela (build, captura de dados, publicação) estão em `extension/CLAUDE.md`.
- **O cofre fala com o painel, nunca com o Go.** A extensão manda a sessão do painel como `Authorization: Bearer` para as rotas `/api/extension/*` do Next, que aceitam **só** o Bearer, nunca o cookie: sem credencial automática, nenhuma página consegue usar a sessão. Essas rotas respondem CORS apenas para origens `chrome-extension://`/`moz-extension://` (ver `frontend/src/lib/extension-relay.ts`). Cada senha lida vira uma linha em `account_credential_accesses`, e o endpoint tem limite próprio por usuário (`CredentialRateLimit`, 30/min).
- `make ext-build` — testes + build dos dois navegadores em `extension/.output/`. `make ext-dev` — build com hot reload.
- **Publicar nas lojas (Chrome Web Store / AMO) só sob pedido expresso**, como o deploy.

## Backup do Banco
- Existe backup automatizado: um **systemd timer** roda `backend/scripts/db-backup.sh` diariamente às 03:30 na máquina de produção (`deploy/systemd/`). Runbook completo em `docs/backup.md`.
- `make db-backup` — dump agora. **Rode antes de qualquer migration destrutiva ou deploy arriscado.**
- `make db-backup-list` — lista os dumps.
- `make db-restore FILE=<caminho>` — restaura num **banco de inspeção** paralelo; produção não é tocada. `MODE=overwrite` restaura por cima do banco vivo (pede confirmação, para API e bot antes).
- Os dois scripts rodam `pg_dump`/`pg_restore` **de dentro do container do banco**, para o client nunca divergir da versão do servidor. Não reescreva para usar binário do host.
- Para recuperar poucos registros apagados por engano, o caminho é o banco de inspeção + `COPY` entre os dois bancos — a receita testada está em `docs/backup.md`. Nunca sugira `MODE=overwrite` para isso: perderia tudo que foi escrito depois do dump.

## Fluxo de Trabalho (Importante)
- Se a tarefa envolver apenas frontend, navegue para `/frontend` antes de codar.
- Se a tarefa envolver apenas backend, navegue para `/backend` antes de codar.
- Antes de trocar de contexto (frontend ↔ backend) com trabalho em andamento, rode `git stash` para não misturar mudanças.
- Não altere a pasta `.github` e não crie pipelines de CI/CD a menos que expressamente solicitado.
- Se for comentar código, comente em inglês e explique o "porquê" da decisão, não apenas o "o quê".
- Toda variável, função, componente ou arquivo deve ter um nome descritivo, claro e em inglês. Evite abreviações obscuras.

## Git e Fluxo de Trabalho

### Staging e Commits
- **NUNCA** use `git add .` ou `git add -A`. Adicione arquivos explicitamente:
  ```
  git add backend/internal/api/v1/handlers/user.go
  git add frontend/src/components/auth/LoginForm.tsx
  ```
- **Padrão de Commit:** Use sempre **Conventional Commits** com escopo. Mensagens em inglês, prefixo obrigatório:
  ```
  feat(auth): add JWT refresh token endpoint
  fix(ui): correct form validation on empty email
  chore(deps): bump go-chi to v5.1.0
  refactor(bot): extract command parser to separate function
  docs(api): add swagger annotations to user handlers
  ```
- **Timing de Commit:**
  1. Após concluir uma tarefa atômica e funcional.
  2. Sempre que os testes (se existirem) passarem.
  3. Antes de iniciar uma refatoração grande, commit o estado estável anterior.
- **Commits atômicos são obrigatórios** — a cada alteração relevante e funcional, gere um commit imediatamente seguindo o padrão Conventional Commits. Nunca inclua co-authorship do assistente nas mensagens de commit.

### Hook local (lefthook)
- O repo usa **lefthook** pra rodar `goimports` + `golangci-lint --fix` nos `.go` staged e `gitleaks` no que está staged a cada `git commit`. Arquivos formatados são reautomaticamente re-adicionados ao stage; lint não autocorrigível (a maioria de `gosec`/`bodyclose`/`contextcheck`) **bloqueia o commit** até ser corrigido manualmente.
- Setup do clone, falha por versão do Go (`go.mod requires go >= ...`) ou hook que não roda (`Can't find lefthook in PATH`, que deixa o commit passar sem lint): ver a skill `lefthook-setup` (`.claude/skills/lefthook-setup/SKILL.md`).

### Quality gate (CI)
O `ci.yml` roda em todo PR para a `main` **e** antes de todo deploy (o `deploy-vps.yml` o chama), então o que reprova aqui não vai ao ar:
- **Go:** gofmt, vet, `golangci-lint` (versão pinada; linters em `.golangci.yml`), testes com Postgres, `govulncheck`.
- **Painel:** `pnpm lint` (as regras do React Compiler são erro: código novo não ganha `eslint-disable` sem motivo escrito na linha), `pnpm typecheck`, `pnpm audit --prod` em high/critical.
- **Extensão:** typecheck, testes, `pnpm audit`.
- **Python:** `ruff` (pinado), testes do azul-login-service, `pip-audit`, sem exceções.
- **Secrets:** `gitleaks` no histórico inteiro; não-secret conhecido fica no `.gitleaksignore` por fingerprint, com motivo.
- O Dependabot (`.github/dependabot.yml`) abre as atualizações semanais; cada uma é um PR comum, passa pelo mesmo gate e só vai ao ar com label.

### Checklist Pré-Commit
Antes de qualquer commit, verificar mentalmente:
- [ ] O build compila? (`go build ./...` ou `npm run build`)
- [ ] Os testes passam? (`go test ./...`)
- [ ] Nenhum `.env`, secret ou credencial foi incluído no staging?
- [ ] Os arquivos adicionados são apenas os da tarefa atual?

### Gestão de Branches
- **Verificação de branch obrigatória no início de cada sessão** — antes de qualquer edição, execute `git branch --show-current`. Se o branch atual for `main`, proponha uma branch dedicada seguindo a convenção de nomes do projeto e aguarde aprovação antes de prosseguir.
- **NUNCA** crie novas branches sem perguntar ao usuário primeiro.
- Padrão de nomes: `feat/nome-da-feature`, `fix/bug-nome`, `hotfix/ajuste-rapido`, `docs/descricao`.
- Exemplo: `git checkout -b feat/whatsapp-group-sync`

### Workflow de PR
1. Branch dedicada → commits atômicos → abrir PR para `main`.
2. Título do PR segue o mesmo padrão de Conventional Commits.
3. **`git push` e abertura de PR SEMPRE precisam de aprovação explícita do usuário** — nunca executar por iniciativa própria.
