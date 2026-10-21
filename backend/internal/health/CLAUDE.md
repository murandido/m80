# M-80 Backend - Sondas de saúde

> **⚠️ Em produção desde 08/08/2026.** As luzinhas passaram a ser o único aviso que a equipe recebe quando a Traveos ou o bot caem — o episódio que originou este pacote agora aconteceria com clientes esperando. Sonda que passa a mentir (verde com a dependência caída) é pior que sonda nenhuma, e o CI não pega isso.

Este pacote responde uma pergunta só: **as dependências externas do processo estão no ar agora?** O painel usa a resposta pra acender as luzinhas do header, visíveis pra qualquer usuário autenticado.

Ele existe por causa de um episódio concreto: a Traveos mudou o formato do token e renomeou um campo, a integração ficou dias fora do ar, e o único sintoma que chegava na equipe era ⚠️ solto no WhatsApp e 502 no "Repuxar" — indistinguível de "essa emissão específica deu errado". O CI ficou verde o tempo todo.

## Por que o pacote não conhece Traveos nem whatsmeow

`monitor.go` recebe cada dependência como `Probe` (`func(ctx) (detail string, err error)`) e não importa nada de domínio — mesmo espírito de `internal/events`. É isso que faz o teste dele rodar **sem Postgres e sem rede**, e o que impede o pacote de virar depósito de regra de negócio.

As sondas concretas ficam em `probes.go`, o único arquivo que pode conhecer `carriers/traveos` e o `BOT_INTERNAL_URL`.

A forma (snapshot atrás de lock + callback `notify`) é a mesma que `internal/bot/status.go` já usava pra sessão do whatsmeow, pelo mesmo motivo: o pacote não pode saber o que é HTTP nem o que é SSE.

## `TraveosProbe` chama `ActiveAirlines`, não `TenantID`

O client cacheia o token. Uma sonda que só autenticasse responderia do cache e diria **"no ar" com a vendor caída** — que é quase exatamente a falha que originou este pacote. Bater num endpoint real exercita o token, o roteamento e o banco deles numa leitura barata.

O parâmetro é o `*traveos.Client` concreto, **não uma interface**: um `*Client` nil guardado numa interface não é nil, e essa checagem passando batido é uma armadilha que `internal/bot` já teve que documentar uma vez.

## `not_configured` não é `down`

Servidor que nunca recebeu credencial da Traveos é um estado suportado (`cmd/api` sobe assim de propósito). Pintar isso de vermelho ensina todo mundo a ignorar a luz. Sonda devolve `ErrNotConfigured` → estado próprio, cinza no painel.

## `notify` só na transição

O `notify` do monitor está ligado a `broker.Broadcast(events.Event{Type: "system.status"})`. Se ele disparasse a cada tique, seria um evento SSE pra todo browser aberto centenas de vezes por dia sem novidade nenhuma. Por isso `CheckedAt` **não** entra na comparação — ele avança em todo tique por definição. Só `State` e `Detail` contam.

`Detail` existe pro bot: o processo pode estar vivo **e** deslogado do WhatsApp, e uma coisa não implica a outra. Mudança de sessão (`connected` → `disconnected`) é notícia mesmo com o `State` parado em `up`.

## O que nunca pode sair por aqui

O `/status` interno do bot devolve `pair_code` e `jid` junto do estado. A sonda lê **só o estado**, e `schemas.DependencyStatus` não tem campo pros outros dois.

O `pair_code` é credencial: quem lê parea o WhatsApp da empresa no próprio aparelho. Ele continua exclusivo do proxy admin-only em `internal/api/v1/routes/bot.go`. `GET /api/v1/status` é aberto a **qualquer** usuário autenticado — não "complete o DTO".

Mensagem de erro da vendor também não sai. A Traveos responde quase toda falha com o mesmo 500 opaco (ver `carriers/traveos/errors.go`), então qualquer motivo na tela seria inventado. Vai pro log.

## Companhias aéreas: duas fontes, a mais recente vence

Desde a camada de provedores (`internal/booking`) o `Snapshot` tem um terceiro eixo: `Providers`, um mapa por **nome de provedor** (`azul`, `gol`), não por companhia — um provedor pode servir mais de uma cia, o endpoint da GOL responde por reserva vendida como Smiles também.

`Bot` e `Traveos` continuam campos fixos e as cias são mapa, e a assimetria é de propósito: aquelas duas são ligadas por nome no `cmd/api`, então um typo tem que ser erro de compilação; as integrações de cia são registradas no boot a partir de uma lista que cresce e não dá pra nomear aqui de antemão.

O status de cada uma vem de **duas** fontes:

- **`Observe`** — resultado das buscas reais, reportado pelo `Registry.Watch`. É o sinal mais forte: uma cia que está sendo usada o dia inteiro responde perguntas de verdade a cada poucos minutos.
- **`RunProvider` + `BookingProviderProbe`** — sonda sintética num ticker lento, pras cias em que ninguém consultou nada hoje. Ela pergunta por uma reserva que **não pode existir** e lê o formato da recusa: `ErrNotFound` é a cia **no ar** (o request chegou na aplicação dela), `ErrUnavailable` é fora. Não existe aqui um catálogo barato como o `/airlines/active` da Traveos; essa é a única sonda possível sem gastar a consulta de alguém.

**`Get()` copia o mapa.** Devolver o vivo entregaria a cada request HTTP uma referência que as sondas seguem escrevendo, e o lock solto na saída não estaria protegendo nada.

## Intervalos

| Dependência | Intervalo | Por quê |
|---|---|---|
| bot | 60s | chamada dentro da rede do Compose, custo zero |
| Traveos | 120s | request de verdade na produção de outra empresa |
| cia aérea | 10min | request na borda pública da cia, que é justamente o que essa integração depende de não ser notada por — e as buscas reais já mantêm a luz fresca |

Timeout de 10s por sonda (`probeTimeout`): sonda pendurada não pode continuar pendurada quando o próximo tique chegar.
