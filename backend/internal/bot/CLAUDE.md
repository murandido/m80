# M-80 Backend - Domínio do Bot

> **⚠️ O bot está em produção desde 08/08/2026**, pareado a um número de WhatsApp real e escutando grupos com gente dentro. É o único componente do sistema que **fala com pessoas** e que **gasta emissão de verdade na Traveos** — as duas coisas não têm desfazer.
> - Bug aqui vira mensagem errada num grupo de cliente, ou uma emissão real numa requisição não-idempotente. Ao mexer em `handlers.go`, `commands.go`, `actions.go` ou no parser da EMISSAO, prove o caminho com teste antes de publicar.
> - **A sessão do whatsmeow vive no banco.** Perder o volume ou restaurar dump por cima do banco vivo desloga o bot e exige reparear no aparelho. Ver `docs/backup.md`.
> - `SetDelaysEnabled` continua sendo só de teste. Desligar o humanizador em produção é exatamente o comportamento de robô que faz um número ser banido.

> **Contexto de produto:** o comportamento pretendido do bot — formato das mensagens, workflows de EMISSAO/ALTERACAO/CANCELAMENTO/DELETAR, fila por chat, lock por PNR, humanizador e integração Traveos — está no Documento de Decisões e Requisitos, no Notion (link em `backend/carriers/CLAUDE.md`).
> **Atenção — o doc do Notion está atrasado em relação ao código.** Ele marca 🔴 "não implementado" para `ALTERACAO`, `CANCELAMENTO`, `DELETAR` e edição nativa; os quatro existem. A fila por chat, o lock por PNR, a deleção nativa, a busca de itinerário na Traveos e o **modo PDF** também passaram a existir — e o `!pdf` do código **alterna** o estado, ao contrário do "toggle cego descartado" registrado lá. O doc também descreve um 🟡 removendo a reação 🕐 — ela foi reintroduzida por decisão posterior, ver *Como o bot responde*, que também documenta a reação ⚠️, nova e não descrita lá. Confirme no código antes de assumir qualquer coisa.

## Arquitetura do Bot
- O bot processa mensagens e dispara automações via WhatsApp.
- Integração WhatsApp: `go.mau.fi/whatsmeow` — o cliente é inicializado no entrypoint `/cmd/bot` e injetado nas camadas internas.
- Padrão de Camadas:
  - `handlers.go` — recebe eventos do WhatsApp (via `client.AddEventHandler` do `whatsmeow`), enfileira, e depois admite (`admit`) e executa (`executeJob`). Não é um servidor HTTP: o bot mantém uma conexão persistente, não recebe webhooks.
  - `queue.go` — a fila por chat. `commands.go` — parse de comandos via `switch/case`.
  - `pnr_lock.go` — registro dos jobs em andamento e lock por PNR.
  - `actions.go` e `service.go` — executam lógicas de negócio e integram com o banco de dados.
- `humanizer/` — pacote isolado com os atrasos aplicados só na camada visível no WhatsApp (marcar como lida, reagir, "digitando", resposta final). Nunca deve ser estendido para atrasar fila, lock por PNR, chamada à Traveos ou os gatilhos que pulam o pipeline de propósito (`DELETAR` e a deleção nativa).
  - **Os atrasos ficam desligados nos testes de `internal/bot`**, via `humanizer.SetDelaysEnabled(false)` no `TestMain` (`main_test.go`). Eles respondiam por ~90% do tempo do pacote — tempo em que nenhum teste aprende nada — e chegaram a estourar o timeout padrão de 10 min do `go test`. **`SetDelaysEnabled` existe só para teste; produção nunca chama** — desligar o humanizador é exatamente o comportamento de robô que ele existe para evitar.
  - As faixas continuam aferidas com tempo real por `internal/bot/humanizer/humanizer_test.go`: é **outro pacote**, logo outro binário de teste, que o flag não alcança. E o `TestHumanizedFlow_...` religa os atrasos para si mesmo, porque vê-los acontecer é o propósito dele. Teste novo que dependa do tempo do humanizador faz o mesmo — não mexa no `TestMain`.

## Fila por chat
Cada chat tem **duas goroutines**, criadas na primeira mensagem daquele chat e derrubadas por `Bot.Shutdown` (chamado por `cmd/bot` antes do `Disconnect`):

- **intake** — ordem de chegada garantida. Desembrulha a edição, classifica, aplica o gate e entrega ao worker. Não dorme: tudo que o grupo vê (marcar como lida, ✅, 🕐) sai numa goroutine própria de cada mensagem, esperada via `job.acked`. Se o atraso do humanizador ficasse aqui, um burst de 20 mensagens empilharia 20 atrasos antes de a última ser olhada.
- **worker** — executa um job por vez, na ordem em que o intake admitiu.

`EventHandler` faz um **send não-bloqueante** no canal de intake: ele roda na goroutine de dispatch do whatsmeow e não pode bloquear nem fazer I/O. É esse append que preserva a ordem — antes havia uma goroutine por mensagem, cada uma dormindo 0.5–1.5s aleatórios antes de qualquer coisa, então uma `ALTERACAO` conseguia chegar ao banco antes da `EMISSAO` que ela corrige.

Chats são independentes entre si de propósito: um grupo lento não pode travar os outros, e nada entre dois grupos precisa de ordem. O que cruza chats — duas mensagens sobre o mesmo PNR — é guardado pelo lock.

**Pulam a fila:** `DELETAR` e a deleção nativa. As duas existem para interromper trabalho que está na fila ou em execução; esperar atrás dele significaria chegar só depois que ele terminou.

**Nada disso persiste.** Um restart perde o que estava enfileirado — decisão registrada no Notion (fila distribuída com Redis/RabbitMQ foi descartada: "volume real não justifica"). O índice único parcial `UNIQUE (program_locator) WHERE active = true AND deleted_at IS NULL` continua sendo a rede contra duplicata.

## Lock por PNR e registro de jobs (`pnr_lock.go`)
O `jobRegistry` indexa o trabalho em andamento de duas formas: **por message id** (para a edição/deleção nativa achar o job que aquela mensagem iniciou) e **por localizador** (para o `DELETAR` e para o lock). As duas entradas somem quando o job termina — o mapa é proporcional ao trabalho em voo, não ao histórico. O `issued` (`issued_messages.go`, teto 500) é o índice equivalente para o que **já** foi gravado; os dois convivem.

O lock é tomado quando o job **começa**, não quando é admitido. Duas mensagens sobre o mesmo PNR no mesmo chat já são serializadas pelo worker daquele chat, e recusar a segunda ali quebraria justamente a ordem que a fila existe para dar. O que o lock pega é o mesmo PNR chegando em **dois chats ao mesmo tempo** → ❌ "A emissão X já está sendo processada".

**Ordem de lock, onde os dois são segurados: `jobRegistry.mu` primeiro, depois `job.mu`.** Nenhum caminho faz o contrário.

Todo job roda com um `context.Context` cancelável. Os handlers recebem esse `ctx` como primeiro parâmetro e rodam suas transações com `b.DB.WithContext(ctx)` — é isso que torna o cancelamento real: puxar o contexto aborta o statement e desfaz a transação. O mesmo `ctx` vale para **tudo que o grupo vê** (`ReactMessage`, `ReplyMessage`, `SendPresenceTyping`): um job cancelado não pode continuar digitando no grupo depois que quem o cancelou já respondeu. As reações de entrada (✅/🕐) são a exceção deliberada e rodam em `context.Background()` — elas acontecem antes do job começar, e um cancelamento posterior não pode engolir o que o remetente já viu.

**`refuse` não responde nada quando o `ctx` já foi cancelado**: quem cancelou é quem deve a resposta ao grupo, e "Não consegui registrar essa emissão" seria mentira. Mas o silêncio total deixava o 🕐 pendurado, porque a resposta de quem cancelou vai numa **outra** mensagem. Por isso `cancel` recebe um parâmetro dizendo se a mensagem interrompida ainda precisa de resposta:

- **`DELETAR` → `true`.** O worker fecha a mensagem interrompida com **❌ sem texto** — o único lugar onde ❌ vem sozinho. O operador já sabe o motivo (foi ele que pediu); o que ele não veria é *qual* mensagem pendente o DELETAR pegou.
- **Edição de job em execução → `false`.** A edição é processada no lugar do job e reage de novo na mesma mensagem; um ❌ ali piscaria uma recusa de um trabalho que vai ser refeito.
- **Deleção nativa → `false`.** Não sobrou mensagem para reagir.

## Regras de Camadas
- Handlers **não** devem conter lógica de negócio — apenas receber, validar superficialmente e delegar.
- `actions.go` e `service.go` **não** devem depender de `*http.Request` — as camadas internas são agnósticas a HTTP.
- Evite misturar conceitos REST nesta camada, exceto no momento exato de receber o evento do WhatsApp.

## Como o bot responde
Reagir é a resposta padrão; mandar texto é exceção. O grupo é onde a equipe trabalha, e cada mensagem do bot é ruído no scrollback de todo mundo.

**Uma reação substitui a anterior do mesmo remetente — o WhatsApp guarda uma por remetente por mensagem, não empilha.** Então ✅ → 🕐 → 🚀 é **um** indicador de progresso mudando de glifo, não três marcas.

| Reação | Significa | Quando |
|---|---|---|
| ✅ | Vi essa mensagem | Assim que o gate libera, antes de qualquer trabalho |
| 🕐 | Está na fila, esperando a vez | Logo depois do ✅, exceto nos gatilhos que pulam a fila |
| ⏳ | Estou consultando a Traveos agora | Início da busca de itinerário, nos dois gatilhos (EMISSAO e correção que redispara) |
| 🚀 | Entendi e executei | Fim de tudo que deu certo — **inclusive o que não mudou nada** |
| ⚠️ | Gravei, mas não consegui completar | A emissão foi salva e a Traveos não devolveu o itinerário — ver *Busca na Traveos* |
| ❌ | Recusei, e o texto diz por quê | Único caso que manda mensagem — **exceto** quando fecha um job interrompido, aí vai sozinho |

- **🚀 inclusive quando nada mudou**, porque "o que você pediu já é verdade" é sucesso. Vale para uma correção que não move campo nenhum e para um `!registrar-chat` duplicado. Só o que muda de fato grava `audit_logs` e dispara SSE.
- **⏳ separa "na fila" de "trabalhando".** A chamada à Traveos leva dezenas de segundos e tem 120s de teto — muito mais que qualquer outra espera do grupo. Com o 🕐 no lugar durante todo esse tempo, a leitura é "o bot travou e a emissão está presa na fila pra sempre". O relógio é espera; a ampulheta é trabalho em andamento. Sai do `syncTraveos`, não dos gatilhos, pra que nenhum caminho anuncie uma chamada que não vai acontecer — e o desfecho (🚀/⚠️) sempre a substitui, então ela nunca fica pendurada.
- **⚠️ não manda texto, e isso é deliberado.** A Traveos responde quase toda falha com o mesmo 500 opaco — localizador errado, sobrenome errado, site da cia fora do ar e bug interno dela são indistinguíveis de fora —, então qualquer motivo que o bot escrevesse no grupo seria inventado. O motivo real vai pro log; o operador vê a linha parcial na grid e o botão de repuxar a um clique.
- **❌ é o único que manda texto**, porque um emoji não consegue dizer *qual* dos cinco números da linha 3 estava errado.
- **Nenhuma reação** em três casos: gate reprovado (chat não aprovado ou remetente desconhecido — quem digita num grupo onde o bot está de passagem não pode descobrir que ele lê); mensagem que o bot não responde (conversa normal, comando inexistente); e a **deleção nativa**, onde a mensagem que receberia a reação é exatamente a que sumiu. Os três vão pro log.

**Consequência de reagir cedo:** tudo que é admitido *precisa* terminar numa reação, senão o 🕐 fica pendurado para sempre. Por isso `classify` recusa na entrada o que o handler poderia decidir ignorar (comando desconhecido, edição de conversa normal), e por isso os ramos silenciosos do `!registrar-chat` passaram a responder — 🚀 no duplicado, ❌ no chat desativado.

## Comandos
- Todos os comandos começam com `!` (ex: `!ping`, `!registrar-chat`, `!pdf`).
- O parse acontece em `commands.go`. Ao adicionar um novo comando, adicione um `case` no switch principal seguindo o padrão de `cmdPing`.
- Comandos desconhecidos devem retornar uma mensagem de ajuda amigável, nunca um erro silencioso.
- **Adicionar comando exige as duas pontas:** o nome em `cmdName*` **e** um `case` no switch. Só a primeira e o 🕐 fica pendurado sem resposta; só a segunda e o intake nem admite a mensagem.

### `!pdf` — modo PDF por chat
- `!pdf` sozinho **alterna** o estado; `!pdf on` / `!pdf off` escrevem estado explícito, pra quem quer o resultado final sem depender do atual. O toggle já foi descartado uma vez (ninguém no grupo enxerga o estado antes de digitar) e voltou por decisão do usuário: a resposta em texto diz em que estado o chat ficou, então uma alternância na direção errada é visível na hora e um segundo `!pdf` desfaz.
- Argumento que o bot **não consegue ler** (`!pdf sim`) é o único caso que ainda só **mostra o estado**: ali existe uma intenção que o bot falhou em ler, e alternar poderia ser o oposto do pedido. `!pdf` sem argumento nenhum não tem essa ambiguidade.
- As quatro formas vivem em `parsePDFIntent` (`pdf_mode.go`), que devolve `toggle` / `on` / `off` / `unreadable` — "não falou nada" e "falou algo ilegível" são valores separados de propósito.
- Junto de `!ping`, é o único comando que responde com **texto** — emoji não diz "ligado" nem "desligado". O 🚀 vem junto pra fechar o 🕐.
- Só funciona em chat com `status='approved'` e `active=true`. Fora disso ❌ **com texto**, e não em silêncio como o gate da EMISSAO: `!pdf` é dirigido ao bot e vem de usuário conhecido, então o remetente já sabe que ele lê.
- Grava `listened_chats.pdf_mode`, a **mesma coluna** que o toggle do painel (`PATCH /bot/chats/{id}/pdf-mode`). Os dois lados escrevem estado explícito, nunca toggle, justamente porque o outro pode ter acabado de escrever.

### Cadastro em promoções (`promo.go`)
Com `promo_mode` ligado no chat, o bot cadastra os CPFs da agência nas promoções de transferência de pontos. **Um gatilho só: o link solto.** Existia um `!promo <link>` e ele foi aposentado — era cerimônia em volta do mesmo link, e a promoção é colada no grupo de qualquer jeito.

- **Três degraus antes de qualquer reação**, todos em `mayAct`: remetente conhecido, chat aprovado e ativo com `promo_mode` ligado, e **domínio coberto por alguma receita ativa**. Reprovar em qualquer um deles é silêncio total — sem ✅, sem 🕐, sem resposta —, e é assim de propósito: sem o comando, o link solto é o único gatilho, e responder a toda promoção compartilhada no grupo seria ruído. O terceiro degrau usa `promotion.ResolveRecipe`, que já sabe casar subdomínio.
- **`isPromoLinkMessage` é estrito e isso é a feature:** a mensagem tem que *ser* o link. Link no meio de uma frase é alguém comentando uma promoção, e cadastrar nisso gastaria CPFs em conversa. E `classifyText` checa promoção **por último**, para um link dentro de uma EMISSAO nunca tomar a mensagem.

- **`kindPromo`, não `kindCommand`**, e o motivo é um só: ele **pula a fila do chat**. Um cadastro custa ~12s por CPF, então uma lista de dez segura a fila por dois minutos e toda EMISSAO digitada nesse meio tempo esperaria atrás dele. Não toca PNR, então também não toma o lock por localizador.
- **Reage ⏳, nunca 🕐.** Pular a fila já faz o intake não pôr relógio, e relógio seria o fato errado: não é espera, é trabalho. Mesma ampulheta da busca na Traveos, pelo mesmo motivo — dois minutos de silêncio se leem como travamento.
- **Sucesso parcial é ⚠️, nunca 🚀.** Um CPF que ficou de fora de um bônus de transferência é exatamente o prejuízo que a feature existe para evitar; arredondar isso para sucesso seria reintroduzi-lo. A mensagem lista quem não entrou.
- **Link solto é lido com rigor**: só conta mensagem que **é** o link. Link no meio de uma frase é alguém comentando a promoção, e cadastrar aí gastaria CPF em conversa. `classifyText` checa isso **por último**, então link dentro de uma EMISSAO nunca sequestra a mensagem, e o `mayAct` exige `promo_mode` antes de um link solto significar qualquer coisa.
- **O comando explícito responde mesmo com o modo desligado** — foi dirigido ao bot por usuário conhecido, então silêncio se leria como bug. Link solto em chat sem `promo_mode` é ignorado em silêncio, como qualquer conversa.
- **Nada é chutado.** Sem receita para o domínio, ou sem formulário na página, o bot **recusa e diz por quê** (qual domínio, que existe o botão *Testar* no painel). Um chute que "funciona" é pior que uma recusa: ninguém percebe.
- **A idempotência é nossa**, não do site. `promo_enrollments` é a única fonte de verdade sobre quem já está cadastrado — a campanha da Azul contra a qual isso foi construído aceita o mesmo CPF repetidas vezes e responde sucesso todas elas. O link é canonizado sem query string (`promotion.CanonicalURLKey`), senão o mesmo link com `utm_*` diferente recadastraria todo mundo.
- Grava `listened_chats.promo_mode`, a **mesma coluna** do toggle do painel (`PATCH /bot/chats/{id}/promo-mode`). Os dois escrevem estado explícito, nunca toggle.
- A regra de negócio mora em `internal/promotion` (mesmo motivo de `internal/issuance`: a API também precisa dela, para o dry-run do painel). O browser mora em `carriers/cmd/promo`, outro processo — Chrome caindo não pode derrubar a sessão do WhatsApp.

## Envio de imagem (`SendImage` em `actions.go`)
Os prints de comprovante saem por aqui. Como o documento, os bytes sobem primeiro e a mensagem carrega só as chaves — mas `ImageMessage` exige três coisas que `DocumentMessage` não exige: `Width`, `Height` e `JPEGThumbnail`. **Sem a miniatura o balão fica um quadrado cinza** que muitos clientes nunca resolvem. As dimensões são validadas antes de virar `uint32`: elas vêm do cabeçalho do arquivo, que pode mentir.

## Envio do bilhete em PDF (`ticket_pdf.go`)
Com `pdf_mode` ligado, depois de uma EMISSAO que a Traveos **completou**, o bot renderiza o bilhete e manda o arquivo no chat.

- **Depois da reação, nunca dentro dela.** O grupo já foi informado de que a emissão deu certo. Renderizar e subir arquivo não faz parte dessa resposta, e o `pdf-service` tendo um minuto ruim não pode transformar uma EMISSAO bem-sucedida em ⚠️. Toda falha aqui é linha de log e nada mais.
- **Só em `traveosFetched`.** Sem itinerário não há bilhete — um documento em branco entregue a cliente é pior que documento nenhum.
- **O bot gera o PDF em processo, não chama a rota HTTP.** O doc do Notion previa `GET /api/issuances/{id}/pdf`; essa rota é JWT-gated e o bot não tem usuário, então criar auth pra ele — ou uma rota interna sem auth — é mais superfície que o salto vale. O bot importa **os mesmos dois pacotes** que a rota importa (`internal/ticket` + `internal/utils.GeneratePDF`), então a propriedade que a decisão queria (um gerador só, nunca dois layouts de bilhete) continua valendo. Só o hop HTTP sumiu.
- `loadIssuanceForTicket` é cópia da do handler de propósito: `internal/bot` não pode importar `internal/api` (regra de camadas acima), e um terceiro pacote com seis linhas de `Preload` rende menos do que custa.
- **O arquivo se chama `<LOCALIZADOR>.pdf`** — o da cia quando existe, senão o do programa —, o mesmo nome que `GetIssuanceTicketPDF` dá ao download do painel. Sem prefixo: quem recebe busca por PNR, e um `bilhete-` empilharia todo bilhete sob a mesma letra.
- `SendDocument` (`actions.go`) faz **dois passos**: o WhatsApp não aceita bytes dentro da mensagem, então o arquivo sobe pro servidor de mídia primeiro (`Upload`) e a mensagem carrega só as chaves. Faltando qualquer uma delas, o destinatário recebe um documento que não abre.
- `DelayComposing()` antes do envio, como antes de texto: documento aparecendo sem aviso lê como robô, que é exatamente o que o humanizador existe pra evitar.

## Mensagem EMISSAO (sem `!`)
- `EMISSAO` **não é comando** — é o texto que os operadores já escreviam entre si, então não dá pra reconhecer pelo prefixo. `processIncomingMessage` usa `isEmissaoMessage` (só olha a primeira linha) como filtro barato antes de qualquer trabalho.
- **Gate obrigatório:** `handleEmissao` só age em chat com linha em `listened_chats` com `status='approved'` **e** `active=true`, e só de remetente que existe em `users`. Nenhuma das duas recusas gera resposta visível — quem digita num grupo onde o bot só está de passagem não deve descobrir que ele lê.
- Camadas: `emissao_parse.go` é puro (texto → `emissaoMessage`, zero banco); `emissao_resolve.go` resolve códigos e apelidos no banco e auto-cria registros `complete=false`; `emissao.go` orquestra e responde.
- Resolução e criação rodam **na mesma transação** — conta auto-criada por apelido não pode sobreviver a uma mensagem que falha depois.
- **Texto que vai pro WhatsApp é em português.** Erros seguros de devolver ao grupo são do tipo `operatorError`; qualquer outro vira mensagem genérica + log, pra não vazar erro de banco no chat.
- Formato (linha 5 opcional):
  ```
  EMISSAO - <PROGRAMA>[/<CIA>] - <PNR>[/<PNR2>]
  <IATA_ORIGEM>[ - <SOBRENOME>][ - <NUMERO_DA_COMPRA>]
  <pontos>k * <milheiro> + <taxas> tx = <total>
  <apelido_conta>[ - <apelido_admin>][ (<apelido_agencia>)]
  <codigo_de_observacao>
  ```
  A linha 2 é posicional conforme `airlines.requires_last_name`/`requires_purchase_number`. O `= <total>` da linha 3 **não é coluna**: é checksum, recalculado com a mesma fórmula da grid — se não fechar, a mensagem é recusada.
- **Auto-criação a partir de apelido:** um apelido de conta inédito cria `accounts` + `account_nicknames` + a pessoa homônima (`people` + `person_nicknames`) ligada como **titular** — sem titular o card do painel lê "Conta sem titular" e a conta fica sem nome. `administrator_id` fica **nulo** de propósito: se já nascesse preenchido, o primeiro nome diferente na linha 4 viraria apelido do titular para sempre.
- **Conta arquivada é invisível aqui, e isso é o ponto.** `accounts` e `account_nicknames` são soft-deletados (migration 29) e o painel arquiva os dois na mesma transação, então o apelido de uma conta arquivada se comporta como apelido inédito: nasce conta nova. O caminho é o único do bot até uma conta, então arquivar no painel é o que faz a próxima EMISSAO recomeçar limpa. Apelido vivo apontando para conta arquivada não deveria existir, mas se alguém deixar um pela metade no banco, `resolveAccountByNickname` reaponta o apelido para a conta nova em vez de abortar a mensagem.
- **Administrador de conta existente:** o administrador é fato assentado, então um nome novo na linha 4 vira **mais um apelido dele**, não pessoa nova. Se o nome já pertencer a outra pessoa, os dois sentidos conflitam (`person_nicknames.nickname` é único global) e a mensagem é **recusada** — nunca ignorada em silêncio, que era o bug relatado.
- A transação da EMISSAO grava exatamente **1 `issuances` (`complete=false`) + 1 `sales` + 1 `audit_logs`**. Sem voo e sem passageiro: a mensagem não traz data de voo (`issuance_flights.departure_at` é `NOT NULL`) nem primeiro nome (`issuance_passengers.first_name` é `NOT NULL`). Quem preenche isso é a Traveos, **depois** do commit — ver abaixo.

## Busca na Traveos (`traveos.go`)
Depois que a transação da EMISSAO commita, `syncTraveos` chama `internal/issuance.Sync` para puxar passageiros, voos, assentos e bagagem da Traveos e completar a emissão.

- **Fora da transação, dentro do lock por PNR.** Só a emissão na Traveos já tem timeout de 120s, e segurar uma transação Postgres aberta esse tempo é inaceitável. O lock, a partir daqui, guarda dinheiro e não só consistência de banco: `POST /tickets/emission` não é idempotente, então o mesmo PNR chegando por dois chats ao mesmo tempo emitiria **dois bilhetes reais** na vendor.
- **`force=false` sempre.** O modo `force=true` — "o voo mudou, traz os dados novos" — é exclusivo do botão do painel. Por mensagem, o que existe é primeira busca e retry.
- **A ordem é notificar, buscar, reagir.** O `issuance.created` sai antes de a Traveos ser sequer chamada: a busca pode levar mais de um minuto, e uma grid vazia esse tempo todo parece um bot que engoliu a mensagem. O `issuance.updated` só sai se o itinerário realmente chegou.
- **Reage ⏳ antes de chamar**, depois do check de credencial e antes do `Sync`, pra que a ampulheta nunca prometa uma chamada que não vai sair. Sem humanizador: é indicador de progresso, atrasá-lo anularia o propósito.
- **Três desfechos**, e só um deles é ⚠️ (`traveosOutcome`): *fetched* (🚀, emissão completa), *skipped* (🚀 — `traveos_supported=false` na cia, ou nenhuma credencial configurada no ambiente: nada errado com **esta** emissão), *failed* (⚠️ + log com o erro real, emissão fica `complete=false`).
- **`Bot.Traveos` nulo é estado suportado**, não bug: é o que `cmd/bot` deixa quando falta `TRAVEOS_EMAIL`/`TRAVEOS_PASSWORD`. O campo é tipado como a interface `issuance.TraveosClient`, não como `*traveos.Client` — um `*traveos.Client` nil guardado numa interface **não** é nil, e a checagem do bot passaria batido.
- **Retry pela correção.** Se uma correção (mensagem `ALTERACAO` ou edição nativa — os dois passam por `applyCorrection`) mexer em `airline_id`, `airline_locator`, `origin_iata_code`, `main_passenger_name` ou `latam_purchase_number` **e** a emissão ainda estiver `complete=false`, o bot busca de novo. É o operador consertando exatamente o campo que fez a busca falhar. Qualquer outro campo, ou emissão já completa, não dispara nada. A lista é lida do `details` que vai pro `audit_logs` (`movedATraveosInput`), então o que dispara o retry e o que fica na história nunca divergem.

## Correção: mensagem ALTERACAO e edição nativa
Dois gatilhos, um algoritmo (`applyCorrection` em `alteracao.go`). Camadas iguais às da EMISSAO: `alteracao_parse.go` é puro, `alteracao_resolve.go` toca o banco, `alteracao.go` orquestra, `alteracao_edit.go` cuida do gatilho de edição.
- Formato: `ALTERACAO - <PNR>` na linha 1, e **só as linhas que precisam mudar** depois dela, em **qualquer ordem**. Uma `/` na linha 1 é recusada — o localizador localiza, nunca corrige.
- **Classificação por formato, não por posição.** Contém `*` → valores; primeiro campo com 3 letras → rota; palavra solta sem `-` e sem `()` → observação; o resto → conta/admin/agência.
- **Duas formas são ambíguas no texto e só o banco decide** (`settleAlteracaoLines`): `GRU` vs. `Bia` resolve contra `airports.iata_code`; `ACORDO` vs. `Murilo` contra `observation_presets.code`. Ordem fixa: aeroporto → preset → apelido de conta. Por isso a checagem de duplicata roda **depois** do desempate — uma linha que o parser chamou de rota pode virar a segunda linha de conta.
- Cada linha é lida pela **mesma função que lê a linha equivalente da EMISSAO** (`parseEmissaoAmounts`, `parseEmissaoRoute`, `parseEmissaoOwners`) — um número ou um apelido significa a mesma coisa nas duas mensagens. O checksum `= total` continua obrigatório.
- **Campo ausente numa ALTERACAO significa "não falei disso", nunca "apague".** Vale também para os extras da linha de rota: `assignCorrectionExtras` é a contraparte branda de `assignLine2Extras` e **não** exige o que a cia exige numa EMISSAO — senão corrigir só a origem numa emissão da GOL seria impossível. Campo a mais continua sendo erro. Consequência: pela ALTERACAO não dá pra *limpar* um sobrenome; dá editando a EMISSAO original (que restata tudo) ou pelo painel.
- **Rota em emissão `complete=true`** — esses valores já foram confirmados na fonte, mas os dois gatilhos divergem de propósito: numa **ALTERACAO** a linha é pedido deliberado, então a mensagem inteira é **recusada** (❌ + texto, transação desfeita) — 🚀 diria que a origem mudou; numa **EMISSAO editada** a linha 2 vem porque a mensagem restata tudo, não porque alguém pediu, então é **descartada em silêncio** e o resto aplica normal. Sem isso, corrigir o preço de uma emissão completa por WhatsApp seria impossível.
- Toda correção que muda algo grava **1 `audit_logs` com `action='updated'`** e `details = {"before":…, "after":…}` **só das colunas que se moveram**. Mensagem que não muda nada reage 🚀 mas não grava nem notifica.
- **Edição nativa — dois envelopes aninhados.** `unwrapEdit` (em `handlers.go`) tira os dois e reescreve `Info.ID` pro id da mensagem original; sem isso `utils.GetText` devolve `""` e a reação vai pra um id que nenhum cliente mostra.
  1. `SecretEncryptedMessage`, **lacrado com o message secret da mensagem que está sendo editada** — só quem recebeu a original tem a chave. Abre com `Client.DecryptSecretEncryptedMessage` (por isso `ClientBase` expõe o método). Se não abrir, o bot **não responde nada** e loga erro: responder errado é pior que não responder.
  2. Dentro dele, um `ProtocolMessage` `MESSAGE_EDIT`, cujo `editedMessage` é o texto novo e cujo `key.ID` é a mensagem substituída. O whatsmeow tira esse no caminho de history sync (`ParseWebMessage`) e não no ao vivo — e a edição que vem por history sync **não tem** o envelope de fora, por isso cada camada é opcional.
- Isso roda dentro de `processIncomingMessage`, não no dispatch do evento: abrir o lacre lê o secret gravado, e I/O nunca pode travar a goroutine de dispatch do whatsmeow.
- **Quem decide se é edição é `evt.Info.Edit`** (`isMessageEdit`) — o atributo `edit` do stanza, que o whatsmeow preenche em `parseMessageInfo` antes de descriptografar qualquer coisa: `"1"` edição, `"2"` fixar, `"7"`/`"8"` apagar. É o único sinal que não depende de como o payload veio embrulhado, e é o que separa correção de pin e de delete. Os dois sinais tentados antes não se sustentam: `evt.IsEdit` só é levantado pro wrapper `EditedMessage` (`UnwrapRaw`), e o enum `type` do `ProtocolMessage` é pior que inútil quando ausente — `MESSAGE_EDIT` é 14, então type vazio lê de volta como `REVOKE` (0). O formato ficou só como fallback do history sync, que não traz o atributo. `unwrapEdit` levanta `evt.IsEdit` ele mesmo, pro roteamento a jusante olhar um lugar só.
- **Nenhuma edição morre calada.** Todo stanza com `edit` loga a chegada (`EventHandler`) e toda edição que não vira correção loga o motivo (`handleMessageEdit`), separando "não consegui ler o texto" de "li e não é nossa mensagem". Foi exatamente esse silêncio que fez o bug custar duas rodadas de diagnóstico errado.
- **Editar uma mensagem cujo job ainda não terminou não é correção de nada** — não há nada salvo pra corrigir. É reescrita do trabalho, e o `supersede` (em `handlers.go`) decide como pelos três estados do doc:
  1. **ainda na fila** — o job é reescrito no lugar e a edição não gera job nenhum. Manter o mesmo job é o que preserva a **posição na fila**, e essa posição importa: uma EMISSAO com uma ALTERACAO já enfileirada atrás dela seria jogada para depois da correção, que então não acharia o que corrigir. Reage ✅ → 🕐 de novo, na mensagem original.
  2. **já em execução** — não há posição a preservar, então o job é cancelado e a edição segue como correção normal. Esse caminho registra a emissão quando o rollback chegou primeiro e corrige quando o commit chegou — certo nos dois casos, já que cancelar uma transação tarde demais deixa a linha gravada.
  3. **já salva, ou nunca foi job** — o algoritmo de correção de sempre, sem mudança.
- Editar uma mensagem para algo que não é mensagem de bot cancela o job pendente e responde ❌ dizendo isso — o 🕐 já estava na tela e precisa ser resolvido.
- Uma EMISSAO editada **restata a mensagem inteira**, então linha apagada = campo limpo (`alteracaoFields.Full`). O mapa `issued` (`issued_messages.go`, teto de 500) existe só para distinguir *localizador reescrito* — recusado — de *localizador que ninguém emitiu*. Sem ele, a correção ainda funciona pelo PNR.
- **O painel não grava audit log nas suas próprias edições.** A grid faz autosave por campo, e uma linha de auditoria por célula editada seria ruído; uma mensagem de WhatsApp é uma correção deliberada e completa. Decisão, não esquecimento.

## Mensagem CANCELAMENTO
- `CANCELAMENTO` **não é comando** — mesmo raciocínio de `EMISSAO`/`ALTERACAO`: prefixo barato na linha 1 (`isCancelamentoMessage`), sem `!`.
- Mesmo gate de `authorizeSender` (chat aprovado + remetente conhecido, recusa sempre silenciosa).
- Formato — linha 2 opcional:
  ```
  CANCELAMENTO - <LOCALIZADOR>
  <pts>k[ + <taxa> tx][ + <multa> multa]
  ```
  Sem linha 2, o cancelamento é reembolso total: `refunded_points`/`refunded_taxes` saem iguais ao que a emissão originalmente tinha, sem multa. Com linha 2, `<pts>k` é sempre obrigatório (mesmo `0k`); `tx` e `multa` são independentes um do outro e cada um ausente vira `0`/nulo.
- **Resolve só pelo PNR** — sem apelido, sem linha de conta/agência. `findIssuanceToCancel` usa a mesma query de `findIssuanceToCorrect` (`program_locator = ? AND active = true`), então uma emissão já cancelada ou deletada não é candidata: uma segunda `CANCELAMENTO` para o mesmo localizador responde "não encontrei", não "já está cancelada" (esse segundo erro só é alcançável pelo cancelamento do painel, que busca por id sem esse filtro).
- A regra de escrita em si (`Active=false`, `cancelled_at`, campos de reembolso, reset de `sales.paid` se pago, `audit_logs.action=cancelled`) vive em `internal/issuance.Cancel` — a mesma função que `POST /issuances/{id}/cancel` do painel chama. O bot só resolve a emissão pelo PNR e monta o `CancelInput`; a escrita é idêntica nos dois lugares.
- Sucesso reage 🚀 e chama `notifyIssuanceCancelled` (evento SSE `issuance.cancelled`). Erro reage ❌ com texto, via `refuse`/`refusalText` — mesmo vocabulário de EMISSAO/ALTERACAO.

## Comando DELETAR
- Mesmo não-comando, mesmo gate que `CANCELAMENTO`. Reage **✅ → 🚀/❌, sem 🕐** — ele pula a fila, e um relógio reivindicaria uma espera que não acontece.
- Formato — uma única linha, sem campos extras:
  ```
  DELETAR - <LOCALIZADOR>
  ```
- **Cobre os três estados do localizador numa passada só:** cancela o job que estiver na fila ou em execução para aquele PNR, depois olha o banco e apaga o que encontrar. Se não achar nada **e** tiver cancelado algo, isso é sucesso — a mensagem foi pega antes de virar linha. Se não achar nada e não tiver cancelado nada, "não encontrei".
- **Cancelar primeiro e conferir o banco depois é o que torna a corrida inofensiva.** Um job cancelado um instante tarde demais — depois do commit, antes do retorno — deixa a linha lá, e a conferência acha e apaga. Reportar sucesso só pelo cancelamento teria afirmado que a emissão foi interrompida enquanto ela estava na tabela. O job cancelado é esperado (`awaitFinish`) antes da leitura, porque o rollback chega um pouco depois do statement abortado.
- **Nunca toma o lock por PNR**: ele existe pra interromper quem está segurando o lock, então disputar por ele travaria justamente o comando feito pra destravar.
- **Alvo é sempre a emissão ativa do localizador**, nunca uma cancelada antiga que ainda carregue o mesmo PNR (o índice único libera o localizador para reuso depois de um cancelamento, então pode haver mais de uma linha histórica com o mesmo `program_locator`). `findIssuanceToDelete` usa `active = true`; uma emissão cancelada não é candidata — `DELETAR` nela responde "não encontrei", igual a um localizador que nunca existiu.
- A regra de escrita (soft delete via `deleted_at`, `audit_logs.action=deleted`, apaga a linha de `sales` junto — diferente do cancelamento, que mantém a venda em aberto) vive em `internal/issuance.Delete`, a mesma função que `DELETE /issuances/{id}` do painel chama.
- Só um `DELETAR` que realmente apagou uma linha chama `notifyIssuanceDeleted` (SSE `issuance.deleted`) — interromper algo que nunca foi gravado não é notícia pro painel.

## Deleção nativa de mensagem (`revoke.go`)
"Apagar para todos" desfaz o que a mensagem tinha posto em movimento. Detectado por `Info.Edit == "7"` (o próprio remetente) ou `"8"` (admin do grupo), com fallback pro `ProtocolMessage_REVOKE` do history sync — que exige `key.ID` presente, porque um `ProtocolMessage` sem type nenhum lê de volta como `REVOKE` (é o zero do enum).

**O alvo é `ProtocolMessage.Key.ID`, nunca `Info.ID`** — esse último é o id do stanza de revoke, não da mensagem que sumiu.

**O stanza não carrega texto nenhum.** Os únicos caminhos até uma emissão são os dois mapas que o bot já mantém: o `jobRegistry` enquanto o trabalho está em voo, e o `issued` depois que ele gravou. Isso é um limite duro, não um atalho — não existe PNR no stanza pra procurar. Mensagem mais velha que o `issued` (teto 500), ou anterior ao último restart, fica intocada e o motivo vai pro log.

Mesmos três estados do `DELETAR`, mesma ordem (cancela, espera, confere o banco). A busca no banco é **por id da emissão**, não por localizador: o localizador é liberado pra reuso depois de um cancelamento, então buscar por ele poderia cair numa emissão diferente que hoje carrega o mesmo PNR.

**Não reage nada** — a mensagem que receberia a reação é a que acabou de sumir do chat. Só a falha fala, como uma linha solta no chat (`SendMessage`, sem quote), porque uma emissão que sobrevive em silêncio à deleção da mensagem que a criou é um fantasma que ninguém pensaria em procurar.

**Fora de escopo, de propósito:** desfazer uma `ALTERACAO`/`CANCELAMENTO` cuja mensagem foi apagada. Reverter uma correção já gravada não está especificado em lugar nenhum e não é barato; o bot loga e não faz nada.

## Boas Práticas de Concorrência e Resiliência
- **Nunca bloqueie a goroutine de dispatch do whatsmeow.** `EventHandler` só faz um send não-bloqueante na fila do chat — nada de I/O, nada de sleep, nada de banco. Tudo o mais acontece do outro lado do canal.
- **Uma mensagem admitida sempre termina numa reação.** Se um caminho novo puder decidir não responder, ele tem que ser recusado em `classify` (antes do ✅) ou responder ❌ — senão o 🕐 fica pendurado.
- **Operações longas** (envio em lote, chamadas externas) rodam em goroutines separadas:
  ```go
  go func() {
      if err := sendBulkMessages(recipients); err != nil {
          slog.Error("bulk send failed", "err", err)
      }
  }()
  ```
- **Em caso de erro no processamento de uma mensagem, logar e continuar** — nunca deixar o bot crashar por uma mensagem malformada:
  ```go
  if err := processMessage(msg); err != nil {
      slog.Error("failed to process message", "jid", msg.Info.Sender, "err", err)
      return // segue para a próxima mensagem
  }
  ```
- Use logs estruturados de forma abundante — interações em tempo real são difíceis de debugar sem contexto suficiente.
