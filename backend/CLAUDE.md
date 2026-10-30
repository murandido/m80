# M-80 Backend - Regras Compartilhadas (Go)

> **⚠️ Este backend está em produção desde 08/08/2026** — Oracle Ampere A1 + Caddy, banco com dado real. **A API não tem endereço público**: desde 29/08/2026 o Caddy serve o painel e roteia só `/health` para cá; todo o resto chega pela rede interna do Compose, vindo do container do Next. As regras de cuidado estão no `CLAUDE.md` da raiz; o que mais pega aqui: **migration roda sozinha no boot da API e rollback de deploy não desfaz schema**. Painel e API agora sobem no mesmo SHA, então a janela de dessincronia virou um reload de aba em vez de dias — mas ela não é zero.

## Fundações
- Linguagem: Go — a versão exigida está no `go 1.x.y` do `go.mod`, e ela é **exata, não um piso**: o `go.mod`, o `FROM golang:` do `Dockerfile` e o `go-version-input` do job de vulnerabilidade em `.github/workflows/ci.yml` andam sempre juntos. Ao subir uma, suba as três — se o CI escanear um Go diferente do que as imagens rodam, ele não está medindo o que a gente publica.
- Dois entrypoints Go: `/cmd/api` (REST API) e `/cmd/bot` (automação WhatsApp via `whatsmeow` — cliente persistente multi-device, **não** um servidor HTTP recebendo webhooks). O bot serializa as mensagens de cada chat numa fila própria e guarda um lock por PNR entre chats — ver `internal/bot/CLAUDE.md`. Há um terceiro binário Go, `carriers/cmd/promo` (no submódulo privado), que cadastra CPFs em promoções de transferência — processo separado porque usa browser, que é pesado e às vezes morre, e a sessão persistente do bot não pode cair junto. E um quarto serviço, `/backend/pdf-service` (Python/Flask/WeasyPrint), consumido internamente pela API para gerar PDFs — ver `backend/pdf-service/CLAUDE.md`.
- Banco de Dados: PostgreSQL acessado estritamente via `gorm.io/gorm`.
- Modelos e conexão com o banco residem **APENAS** em `internal/data`. Convenções de modelo (UUID, soft-delete, seeds) estão em `internal/data/CLAUDE.md`.
- Funções utilitárias (telefone, random, whatsapp, respostas HTTP, criptografia AES-256-GCM, geração de PDF) ficam em `internal/utils`.

## Variáveis de Ambiente Esperadas
As seguintes vars devem estar definidas (via `.env` ou ambiente Docker):
- `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` — credenciais do PostgreSQL (sufixadas `_DEV`/`_TEST`/`_PROD` no `.env`/docker-compose, mapeadas para os nomes simples dentro de cada container)
- `JWT_SECRET` — chave secreta para assinar tokens
- `ENV` — `development`, `test` ou `production`. É o que decide se o seed do admin usa credencial fixa e se o Swagger é servido
- `ADMIN_PHONE`, `ADMIN_PASSWORD` — **só em produção**, e só usadas enquanto a tabela `users` estiver vazia. Sem elas nenhum usuário é criado
- `PORT` — porta da API (padrão `8080`)
- `PDF_SERVICE_URL` — URL interna do `pdf-service` (usado por `internal/utils/pdf.go`). Necessária no `api-*` **e no `whatsapp-bot-*`**: com `pdf_mode` ligado o bot renderiza e envia o bilhete ele mesmo, sem passar pela rota HTTP
- `ENCRYPTION_KEY` — chave usada por `internal/utils/crypto.go` (AES-256-GCM) para criptografar campos sensíveis em repouso: senha de conta, documento do passageiro e, desde a migration 38, `people.cpf`. Necessária no `api-*` **e no `whatsapp-bot-*`**: o sync da Traveos grava `issuance_passengers.document` criptografado, e o bot chama esse sync depois de toda EMISSAO. Sem ela a busca falha no último passo, **depois** de já ter chamado a vendor — o log diz `ENCRYPTION_KEY environment variable is not set` e o grupo vê ⚠️
- `CPF_INDEX_KEY` — chave (64 hex, como a `ENCRYPTION_KEY`) do blind index de CPF em `internal/utils/blindindex.go`. É o HMAC que identifica um beneficiário de conta pelo documento sem guardar o documento. Necessária no `api-*` **e no `whatsapp-bot-*`**, porque o sync da Traveos — que o bot dispara depois de toda EMISSAO — é onde o CPF entra. **Não é descartável nem rotacionável sozinha:** trocá-la não dá erro, só faz todo fingerprint já gravado deixar de casar, e o mesmo passageiro volta a abrir um beneficiário novo. Guarde junto do backup
- `BOT_PORT` — porta da API HTTP interna do `/cmd/bot` (padrão `8081`, não publicada no host, só rede interna do Compose)
- `BOT_INTERNAL_URL` — URL interna do `/cmd/bot` usada pelo proxy admin-only `/api/v1/bot/*` em `internal/api/v1/routes/bot.go`
- `API_INTERNAL_PORT` — porta do listener interno do `/cmd/api` (padrão `8090`, não publicada no host) que recebe o callback de status do bot e repassa pro broker SSE
- `BOT_STATUS_CALLBACK_URL` — URL desse listener interno, usada pelo `/cmd/bot` pra notificar mudança de estado da sessão (ver `internal/events` e `cmd/api/internal_listener.go`)
- `BOT_CHAT_REQUESTED_CALLBACK_URL` — URL do mesmo listener interno, usada pelo `/cmd/bot` pra notificar que `!registrar-chat` criou ou reabriu uma linha em `listened_chats` (mesma ponte de `BOT_STATUS_CALLBACK_URL`, evento `listened_chat.requested`)
- `BOT_ISSUANCE_CREATED_CALLBACK_URL` — URL do mesmo listener interno, usada pelo `/cmd/bot` pra notificar que uma mensagem `EMISSAO` gravou uma emissão (mesma ponte, evento `issuance.created` — o mesmo que os handlers de emissão já publicam, então a grid trata igual)
- `BOT_ISSUANCE_UPDATED_CALLBACK_URL` — mesma ponte, evento `issuance.updated`, usada quando o bot **corrige** uma emissão (mensagem `ALTERACAO` ou edição da mensagem original)
- `BOT_ISSUANCE_CANCELLED_CALLBACK_URL` — mesma ponte, evento `issuance.cancelled`, usada quando o bot **cancela** uma emissão (mensagem `CANCELAMENTO`)
- `BOT_ISSUANCE_DELETED_CALLBACK_URL` — mesma ponte, evento `issuance.deleted`, usada quando o bot **deleta** uma emissão (mensagem `DELETAR`)
- `PROMO_SERVICE_URL` — URL interna do `carriers/cmd/promo` (usada por `internal/promotion`). Necessária no `api-*` (botão **Testar** do painel) **e no `whatsapp-bot-*`** (o cadastro em si). Ausente não derruba nada: o client cai no nome de serviço de dev e o bot responde ao link com uma recusa em vez de quebrar
- `PROMO_PORT` — porta do `carriers/cmd/promo` (padrão `8082`, **não publicada em produção**: o Docker escreve as próprias regras de iptables antes das do host, então não publicar é o controle real)
- `CHROME_PATH` — caminho do binário do browser dentro da imagem do promo (`/headless-shell/headless-shell`). Explícito de propósito: um binário ausente falha no start, com mensagem clara, em vez de falhar na primeira campanha
- `BOT_PROMO_ENROLLED_CALLBACK_URL` — mesma ponte dos outros callbacks, evento `promo_enrollment.created`, disparada quando um cadastro em promoção gravou linhas
- **Integrações com terceiros** (cias aéreas, Traveos, promoções, login da Azul) moram em `backend/carriers`, submódulo privado (`make init` na raiz). As envs delas (`PROMO_*`, `TRAVEOS_*`, `AZUL_FIREBASE_*`, `AZUL_USER_AGENT`, `SMILES_USER_AGENT`, `SMILES_PROXY_URL`, `SMILES_LOGIN_SERVICE_URL`, `AVIANCA_*`, `LATAM_PROXY_URL`, `AIRFRANCE_PROXY_URL`, `GOL_PROXY_URL`) e o porquê de cada uma estão em `backend/carriers/CLAUDE.md`. **Nada sobre como cada integração funciona entra neste repositório, que é público.**
- **O itinerário não vem só de uma fonte.** `internal/booking` define a forma neutra de reserva e a porta que cada fonte implementa. Quem responde por uma emissão é decidido por linha em `issuance.resolveProviders`: a integração da própria cia primeiro, quando `airlines.direct_booking_enabled` está ligado e existe provedor registrado, com o adaptador genérico atrás. `issuances.booking_source` grava quem de fato respondeu. **Registrado ≠ ligado**: cia nova sobe no escuro e é ligada pelo painel, sem release no meio. Antes de acrescentar uma companhia, leia `internal/booking/CLAUDE.md`
- `AZUL_SYNC_ENABLED` — liga o **ticker periódico** de sincronização (`internal/azulsync`, `cmd/api`). Default off. **A primeira sync no connect roda independente dessa flag** — conectar é ação explícita e já puxa até um mês; a flag governa só o polling de fundo
- `AZUL_SYNC_INTERVAL` — intervalo do ticker como duração Go (ex.: `10m`); vazio usa o padrão
- **Chave recusada (`needs_reconnect`).** Quando o `points/resume` responde 404, a Azul não reconhece mais o `customerKey`. Na prática isso acontece quando o titular troca a senha. Nesse caso a conta vai para `needs_reconnect` e o ticker deixa de lê-la (`eligibleAccounts`). Ela ganha **uma** tentativa de login automática com a senha salva (`azulsync.Connector`); se der certo, volta para `pending`. Se falhar, continua estacionada, e só um destes dois caminhos tenta de novo: salvar uma senha **diferente** no cadastro (o `UpdateAccount` dispara o login sozinho) ou clicar em Reconectar. Não há repetição automática de propósito, porque login repetido com senha errada trava a conta TudoAzul. O `Connector` é um só por processo (`cmd/api`), compartilhado pelo botão, pelo ticker e pelo update, para dois deles nunca abrirem dois navegadores para a mesma conta
- `SMILES_SYNC_ENABLED` — liga o **ticker periódico** da Smiles (`internal/smilessync`, `cmd/api`). Default off. Como na Azul, **a primeira sync no connect roda independente da flag**. **Só no `api-*`** (o bot não sincroniza nada)
- `SMILES_SYNC_INTERVAL` — intervalo do ticker como duração Go; vazio usa o padrão
- **Saldo é de toda conta; extrato e estoque próprio são as únicas chaves por conta.** Qualquer conta Azul, Smiles ou LATAM pode ser conectada, e toda conta conectada tem o saldo lido pelo ticker e gravado em `real_balance_points` — sem flag. `accounts.tracks_statement` ("Acompanhar extrato") decide só se a rodada puxa também o extrato (Azul e Smiles; LATAM não tem extrato) e se as telas de extrato abrem; `owns_point_inventory` governa tudo que precifica ponto (lotes, fechamento, custeio FIFO). Numa conta sem extrato a rodada para depois do saldo (`recordBalanceOnly`) e **não** mexe em `last_balance_*` — é o marcador do extrato, e mexer nele faria a primeira rodada depois de ligar o extrato pular a janela como "saldo igual". Ao acrescentar rota de extrato, use o guarda `statementAccount`; ao acrescentar rota de lote, `inventoryAccount`; conectar e o badge de saldo usam `loadAccount`

## Bootstrap em Ambiente Local
- Se a tabela `users` estiver vazia no primeiro start, `cmd/api/main.go` cria automaticamente um usuário admin padrão — consulte o arquivo para as credenciais de desenvolvimento.
- **Isso vale só fora de produção.** Com `ENV=production` o admin sai de `ADMIN_PHONE` + `ADMIN_PASSWORD`; sem as duas, a API sobe **sem usuário nenhum** e diz o motivo em `slog.Warn`. As credenciais fixas de dev estão neste repositório — herdá-las num painel com domínio público seria entregar o admin de graça.

## Padrão de Tratamento de Erros
- Handlers acessam o GORM diretamente (`h.DB.First(...)`, `h.DB.Create(...)`, etc. — não há camada de funções de consulta em `internal/data` hoje) e tratam o erro ali mesmo, convertendo para resposta HTTP com `utils.SendError`.
- **Exceção: criação de emissão.** A regra de negócio vive em `internal/issuance` porque o `/cmd/bot` também precisa dela — ele não tem `*http.Request` pra chamar `utils.SendError`. O pacote devolve erros tipados (`issuance.ErrLoyaltyProgramNotFound`, `ErrProgramLocatorTaken`, ...) e cada consumidor traduz pro seu meio: o handler mapeia pra status HTTP em `issuanceErrorResponse`, o bot pra texto de WhatsApp. Ao adicionar uma regra nova de emissão, coloque-a lá — não no handler.
- Em código de infraestrutura/utilitário onde o erro atravessa mais de uma função antes de virar resposta HTTP (ex: `internal/data/db.go`, `internal/utils/pdf.go`), propague com contexto usando `fmt.Errorf`:
  ```go
  if err := conn.Ping(); err != nil {
      return fmt.Errorf("db.Connect: ping failed: %w", err)
  }
  ```
- Nunca engula erros com `_ =` em código de negócio.

## Logging
- Use logs estruturados; inclua sempre contexto relevante (`user_id`, `request_id`, operação):
  ```go
  slog.Error("failed to send whatsapp message", "user_id", userID, "err", err)
  ```
- Logs abundantes no bot — interações em tempo real são difíceis de debugar sem eles.

## Banco de Dados e Migrations
- O schema é gerenciado por **golang-migrate** com arquivos SQL versionados em `internal/data/migrations/`.
- As migrations rodam automaticamente no startup via `data.Connect()` — antes de o GORM abrir a conexão.
- Para criar uma nova migration: adicionar `NNNNNN_descricao.up.sql` e `NNNNNN_descricao.down.sql` em `internal/data/migrations/`.
- Convenção de nomes: `000001_create_users_table.up.sql`, `000002_create_tickets_table.up.sql`, etc.
- `AutoMigrate` do GORM foi **removido** — não adicionar de volta. golang-migrate é o único dono do schema.
- golang-migrate registra as migrations aplicadas na tabela `schema_migrations` (criada automaticamente no banco).
- Convenções de modelo GORM (geração de UUID, soft-delete seletivo, seeds) estão documentadas em `internal/data/CLAUDE.md`.

## Documentação Swagger
- **A rota `/swagger/*` não é servida com `ENV=production`** (`internal/api/router.go`). Ela fica antes de qualquer autenticação e entrega a superfície inteira da API — em produção isso é um mapa de graça pra quem pedir. Em dev e test continua igual.
- As anotações swaggo ficam nos handlers (`internal/api/v1/handlers/*.go`) e no bloco `@title`/`@BasePath` de `cmd/api/main.go`.
- Os artefatos gerados (`docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml`) são versionados no git e precisam ser regenerados manualmente após qualquer mudança de anotação:
  ```bash
  go install github.com/swaggo/swag/cmd/swag@v1.16.6
  $(go env GOPATH)/bin/swag init -g cmd/api/main.go -o docs
  ```
- Não existe target de Makefile para isso — rode manualmente e commite os três arquivos gerados junto da mudança de anotação, num commit `docs(api): ...` separado.

## Testes
- Escreva testes unitários (`*_test.go`) sempre que adicionar lógicas puras em `internal/utils`.
- Rode com: `go test ./internal/utils/...`
- **Os testes de handlers e de bot dão `t.Skip` em silêncio sem Postgres.** `go test ./...` na máquina local "passa" sem ter rodado nada — a mensagem de skip é a única pista. Antes de abrir PR, rode contra o banco descartável:
  ```bash
  docker compose --profile test run --rm whatsapp-bot-test
  ```
  Esse é o serviço que roda `./internal/...` inteiro; o `api-test` cobre só `./cmd/api/...`. Os dois já rodaram a árvore `internal` inteira, ao mesmo tempo, contra o mesmo banco — era o dobro do trabalho para um único conjunto de resultados.
- **Esse serviço builda uma imagem (`COPY`), não monta volume.** Sem `build`, o Compose reusa a imagem em cache e você roda **código velho** com a saída toda verde — teste novo simplesmente não aparece na lista. Rebuild explícito a cada rodada que precisa valer:
  ```bash
  docker compose --profile test build whatsapp-bot-test
  docker compose --profile test run --rm whatsapp-bot-test
  ```
  Conferência barata: procure o nome do teste que você acabou de escrever na saída (`-v` já está ligado no `command`). Se não estiver lá, a imagem está velha.
- **Todo teste que passa por `runInTx` é paralelo.** O `t.Parallel()` mora dentro do próprio helper (handlers, bot, legacyimport, issuance), e quem paga por ele é o rollback: sob READ COMMITTED uma transação que ninguém commita é invisível para as outras, então testes concorrentes não se enxergam. Duas consequências ao escrever teste novo:
  - **Não chame `t.Setenv` num teste que usa `runInTx`** — o runtime recusa depois de `t.Parallel()`. Env de processo vai no `TestMain` (ver `internal/api/v1/handlers/main_test.go`).
  - **Seed que toca coluna única precisa de sufixo aleatório por chamada**, como os helpers existentes já fazem.
- **Delay de teste é bug de suíte, não custo inevitável.** O humanizador fica desligado no `TestMain` do `internal/bot` (`SetDelaysEnabled(false)`) e o `interruptedJobGrace` do DELETAR é encurtado lá também — três testes esperavam 10s cada por um timeout cujo estouro *é* a asserção. A suíte inteira caiu de ~47s para ~15s com isso mais o paralelismo.
