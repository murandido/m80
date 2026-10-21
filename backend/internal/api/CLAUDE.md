# M-80 Backend - Domínio da API REST

> **⚠️ Esta API serve produção desde 08/08/2026.** Desde 29/08/2026 ela **não tem endereço público**: o Caddy serve o painel e roteia só `/health` para cá, e todo o resto chega do container do Next pela rede interna do Compose. Três consequências diretas neste pacote:
> - **Contrato é compromisso.** Painel e API sobem pela mesma label, no mesmo SHA, então a janela de dessincronia encolheu — mas não zerou: quem está com o painel aberto segue rodando o bundle antigo até recarregar a aba. Renomear campo de resposta, mudar status HTTP ou exigir campo novo no request quebra essa aba. Continue fazendo em dois passos: primeiro o lado que aceita as duas formas, depois o que muda. Só que agora a espera entre os dois é de minutos, não de dias.
> - **Middleware é superfície de ataque, não detalhe.** Não ter endereço público encolheu a superfície, não a zerou: todo request de usuário ainda passa por CORS, rate limit, `RealIP` e auth — só que chegando do painel em vez da internet, o que muda o que "por IP" significa. Leia o item correspondente na *Middleware Stack* antes, e não afrouxe nada sem falar com o usuário.
> - **O cliente desta API é um só.** Não há app de terceiro para quebrar, e também não há ninguém além do painel para exercitar uma rota. Rota que o painel não chama é rota que ninguém testa em produção.

## Arquitetura Base
- Roteador HTTP: `go-chi/chi/v5`.
- Handlers ficam em `v1/handlers/` — devem ser enxutos: extrair JSON, validar, chamar `internal/data`, retornar.
- Schemas de Request/Response (DTOs) residem em `v1/schemas/` e usam tags do `go-playground/validator/v10`.
- Novas rotas devem ser registradas em `v1/routes/` — **nunca** diretamente no `router.go`. Cada arquivo exporta uma função `RegisterXRoutes(r chi.Router, db *gorm.DB)` que cria seu próprio handler e monta `r.Route("/recurso", func(r chi.Router) {...})` com os 5 verbos CRUD (ver `v1/routes/users.go` e `v1/routes/people.go`). `router.go` só compõe os grupos de middleware e chama essas funções.
  - Exceção atual: as rotas de `/auth` ainda estão inline em `internal/api/v1/router.go` (não migradas para `v1/routes/`) — não confundir com `internal/api/router.go`, que só compõe os grupos de middleware.
  - Para restringir uma rota a admins, aninhe um `r.Group` com `middleware.RequireAdmin()` **dentro** do grupo já protegido por `middleware.RequireAuth(db)`. Rotas que qualquer usuário autenticado pode acessar (não só admin) ficam fora desse sub-grupo, como `/people` hoje.

## Middleware Stack
A ordem de execução dos middlewares é (definida em `internal/api/router.go`, com `Auth` aplicado depois dentro de `internal/api/v1/router.go`):
```
RealIP → Logger → Recoverer → CORS → Rate Limiter → Auth (JWT)
```
`RealIP` vem primeiro porque o `Logger` e o rate limiter leem `r.RemoteAddr`, e os dois veriam o container do painel em toda requisição se ele corrigisse depois.
- **CORS:** já configurado no router; **nunca** adicione headers CORS manualmente em handlers. A lista de origens vem de `CORS_ALLOWED_ORIGINS` (separada por vírgula) e **vazia significa nenhuma origem cross-site liberada** — o que basta, porque o browser nunca chama esta API direto: quem chama é o servidor do Next, e requisição servidor-a-servidor não manda `Origin`. Se um dia precisar liberar uma origem, acrescente na env, não no código.
- **Rate Limiting:** `go-chi/httprate` está disponível como dependência — use-o para rotas públicas sensíveis. O fallback por IP é o `keyByRemoteAddr` (`middleware/security.go`), que lê `r.RemoteAddr` diretamente (não headers) e devolve a mesma chave do `KeyByIP` do httprate v0.15 — não volte para `LimitByIP`/`KeyByIP`, deprecados na v0.16. Além do limite geral por usuário, `POST /accounts/{id}/credentials` (cofre da extensão) tem o `CredentialRateLimit`, bem mais apertado: ler senhas em massa é o abuso que ele existe para barrar — não o afrouxe sem falar com o usuário.
- **IP do cliente:** a API não tem endereço público — todo request dela vem do container do painel pela rede do Compose, então `r.RemoteAddr` cru é **sempre esse container**, e os 10/min **por IP** do `LoginRateLimit` valiam 10/min **para a empresa inteira** (um operador errando a senha gasta a cota de todo mundo). O `/health` é o único request que ainda chega aqui via Caddy.
  - Quem repara isso são duas metades. O painel manda o IP: o `fetch` server-side do Next não repassa nada do request do browser sozinho, então o `clientIpHeader` (`frontend/src/lib/client-ip.ts`) anexa **só o elemento mais à direita** do `X-Forwarded-For` que o Caddy escreveu. E o `middleware.RealIP()` (`internal/api/middleware/realip.go`) lê esse header, só com `TRUSTED_PROXY=true`, e também **só o elemento mais à direita** — o proxy *acrescenta* ao final, então tudo à esquerda é escrito pelo cliente.
  - **Não** reintroduza `chi/middleware.RealIP` nem equivalentes: ele lê o elemento mais à esquerda, que é exatamente o bypass do rate limiter (ver GO-2026-5774/5775/5777). Fora de produção o flag fica desligado e a header é ignorada por completo.
- **Auth:** o middleware `RequireAuth` (`internal/api/middleware/auth.go`) valida o header `Authorization: Bearer <token>` — **não** é cookie — via `internal/auth.ValidateToken`, carrega o usuário do banco e injeta no contexto da request.

## Escopo por Agência
Contas `user` e `admin` só enxergam e agem nas agências atribuídas a elas em `user_agencies` (migration 000071); `dev` enxerga todas, inclusive as criadas depois. **Só dev atribui agências, cria/edita agências e dá o perfil `dev`** — admin que pudesse fazer qualquer um dos três estaria definindo o próprio limite.
- O `RequireAuth` carrega o `access.AgencyScope` do usuário a cada request e guarda no contexto; leia com `middleware.AgencyScopeFrom(r.Context())`. **Sem escopo no contexto = nenhuma agência** (fail closed), nunca "todas".
- **Todo endpoint que lê ou escreve dado de agência passa pelo escopo.** Os pontos centrais:
  - Listagens: `scope.Restrict(query, "<tabela>.agency_id")`. Lista vazia vira `1 = 0`, não "sem WHERE".
  - Recurso por id: as rotas `/issuances/{id}/...`, `/sales/{id}/...` e `/clients/{id}` ficam atrás de `middleware.Require{Issuance,Sale,Client}InScope`, que responde **404** (não 403) para registro de outra agência. Rota nova nesses grupos já nasce protegida; rota nova fora deles precisa do mesmo cuidado.
  - Agência escolhida no body (criar/mover emissão, lote, cliente, importação legada, commit de extrato): `scope.Allows(id)`, senão 404.
  - Agência omitida: `access.ResolveDefaultAgency` — a principal se o usuário a vê, senão a primeira dele por nome. O fallback antigo "sempre a principal" jogava o registro numa agência que o usuário pode não enxergar.
- **Cliente pertence a uma agência** (000072) e venda só aponta para cliente da mesma agência da emissão (422 caso contrário). É isso que mantém o pagamento de cliente dentro de uma agência só. Clientes com `agency_id` NULL (os que compravam por mais de uma agência) só aparecem para dev até alguém atribuir — o `NOT NULL` vem numa migration futura, quando o banner do painel zerar em produção.
- **O bot respeita o mesmo escopo** (`authorizeSender` devolve o escopo do remetente): sem isso o WhatsApp seria o caminho em volta de qualquer atribuição. Remetente restrito não cria agência por apelido desconhecido.
- **SSE não é filtrado, de propósito**: os eventos carregam só um id (ou nada) e cada tela refaz a busca com os próprios filtros, que já passam pelo escopo.
- Vazamento aceito: o conflito de localizador (`ErrProgramLocatorTaken`) continua global e revela que o localizador existe em outra agência.

## Respostas HTTP (Regra de Ouro)
- **NUNCA** use `http.Error` ou `json.NewEncoder` diretamente nos handlers.
- Use **SEMPRE** e EXCLUSIVAMENTE:
  ```go
  utils.SendJSON(w, r, http.StatusOK, "mensagem de sucesso", payload)
  utils.SendError(w, r, http.StatusBadRequest, "mensagem de erro")
  ```
  Ambas exigem `*http.Request` (usado internamente pelo `go-chi/render`) e uma mensagem descritiva — a resposta é sempre envelopada em `{success, message, data}` (`utils.StandardResponse`).
- **As exceções são as rotas cujo corpo não é JSON**, e cada uma carrega o porquê num comentário no próprio handler. Hoje são sete: `GetIssuancesExport` e `GetSalesExport` (`.xlsx`), `GetIssuanceTicketPDF`, `GetSupplierReportPDF` e `GetClientReportPDF` (`application/pdf`), `GetIssuanceTicket` (HTML do bilhete) e `EventHandler.Stream` (`text/event-stream`). Um envelope JSON não existe nesses formatos. Não crie exceções novas sem a mesma justificativa — e note que todas devolvem **arquivo ou stream**, nunca um objeto de domínio.

## Tempo Real (SSE)
- `GET /api/v1/events` (`v1/handlers/events.go`) é o **único** canal de tempo real do projeto. WebSocket e polling foram descartados por decisão de arquitetura — não reintroduzir.
- A infra fica em `internal/events` e é **agnóstica de domínio**: `Broker` só faz fan-out de `Event{Type, Payload}`. Módulos novos publicam chamando `broker.Broadcast(...)` — **nunca** adicione conhecimento de domínio ao pacote `events`.
- O broker é criado uma vez em `cmd/api/main.go` e passado adiante manualmente, exatamente como o `*gorm.DB` (`api.NewRouter(db, broker, traveosClient, monitor)` → `v1.NewRouter(...)` → `RegisterXRoutes(r, db, broker)`; nem toda rota recebe os quatro). **Não** use singleton de pacote — global mutável quebra os testes com `-race`.
- Publique **sempre depois** de a transação ter commitado, nunca dentro dela: um evento sobre uma mudança que faz rollback é pior que evento nenhum.
- Payload por convenção é mínimo (`{"id": "..."}`). Quem consome refaz a busca com os próprios filtros — as regras de filtro moram no servidor e não devem ser duplicadas no cliente.
- **`system.status`** é o evento das luzinhas de dependência do header (`GET /status`, `internal/health`). Ele é publicado pelo `notify` do monitor, que **só dispara quando o estado muda de fato** — nunca a cada tique da sonda. Sem essa regra seriam centenas de broadcasts por dia, pra todo browser aberto, sem novidade nenhuma. O `bot.status` continua existindo em paralelo e é outra coisa: é o push imediato da sessão do whatsmeow (inclusive cada refresh de QR code no pareamento). O indicador do header escuta os dois, porque um logout do bot tem que apagar a luz na hora, não no próximo tique de 60s.
- Em testes de stream use `httptest.NewServer` + client HTTP real. `httptest.ResponseRecorder` **não** é seguro para acesso concorrente e acusa race quando o handler escreve de outra goroutine.

## Audit Log
`audit_logs` registra `created`, `cancelled`, `restored` e `deleted` a partir do painel — mas **não** `updated`. A mesma decisão vale para `PUT /issuances/{id}/seats` e `PUT /issuances/{id}/baggage`: corrigir um assento é detalhe, e uma linha de histórico por assento enterraria as entradas que importam. Os dois disparam `issuance.updated` no broker, porque a grid mostra a emissão. O `PATCH /issuances/{id}` da grid é autosave por campo (on blur / debounce ~500ms), então uma entrada por edição significaria uma linha de histórico por célula, enterrando as que importam. Só o bot, a mesclagem de contas e as alterações de passagem (`/issuances/{id}/adjustments`, `details.reason` = `adjustment_created`/`_updated`/`_deleted`) gravam `updated` hoje: uma mensagem de WhatsApp é uma correção deliberada e completa, a granularidade certa (ver `internal/bot/alteracao.go`), e a mesclagem de linhas da tela de contas (`POST /people/{id}/merge-row`, que funde as contas repetidas de um programa) é uma ação única e irreversível que move a emissão de conta — sem a linha, o operador vê o campo ter mudado sozinho. As duas usam `updated` porque `audit_action` é um ENUM do Postgres e acrescentar valor nele é migration que o `down` não desfaz; o motivo real vai em `details.reason`. **É decisão, não esquecimento** — se um dia o painel precisar de trilha de edição, ela tem que vir com agrupamento por sessão de edição, não uma linha por `PATCH`.

## Mapa de Status HTTP do Projeto
| Status | Quando usar |
|---|---|
| `200 OK` | GET, PUT com retorno de dado |
| `201 Created` | POST que cria recurso |
| `400 Bad Request` | JSON malformado, campo inválido |
| `401 Unauthorized` | Token ausente ou inválido |
| `403 Forbidden` | Token válido mas sem permissão |
| `404 Not Found` | Recurso não encontrado no banco |
| `422 Unprocessable Entity` | Validação de negócio falhou (ex: email já cadastrado) |
| `500 Internal Server Error` | Erro inesperado — logar antes de retornar |
| `503 Service Unavailable` | Dependência fora do ar (banco inacessível, timeout). **Nunca use 401 para isso** — o painel apaga o cookie de sessão ao ver 401, então um soluço do banco deslogaria todo mundo. Ver `middleware.RequireAuth`. |
