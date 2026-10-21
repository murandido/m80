# M-80 Backend - Camada de provedores de reserva

> **⚠️ Em produção desde 08/08/2026.** Este pacote fica no caminho de **toda** emissão que puxa itinerário — painel e bot. Ele não grava nada: quem grava é `internal/issuance`. Mas é ele que decide **quem responde**, e um roteamento errado aqui aparece como "o bilhete saiu com dado de outra reserva", não como erro.

Este pacote responde uma pergunta só: **de onde vem o itinerário de uma emissão?**

Ele existe porque o M-80 está saindo da Traveos, uma companhia aérea por vez. Sem uma forma própria de reserva, cada cia nova viraria mais um `if` dentro de `internal/issuance`, e o vocabulário da vendor — `ida`/`volta`, `tripType: "somente_ida"`, um `airlineId` inteiro do catálogo dela — continuaria vazando para código que não tem nada a ver com aquele fornecedor.

## O tipo neutro sai do PDF, não da Traveos

`Booking` carrega o que o **bilhete do passageiro** precisa, porque é para isso que o itinerário é buscado. A lista autoritativa é `internal/ticket.ViewModel`: localizador, trechos, e por passageiro o nome, o assento por perna e as franquias de bagagem.

Tarifa, pagamento, contato, número de bilhete e categoria de fidelidade voltam de quase todos esses endpoints e **nenhum deles é modelado**. Não é esquecimento: nada do nosso lado lê essas coisas, e persistir o meio de pagamento de um passageiro para desenhar um documento de embarque seria guardar dado que não temos uso para.

A exceção é `Document`, e ela não é do PDF: é o que `issuance.registerBeneficiaries` cobra contra o teto de beneficiários da conta. **Vários endpoints de cia mascaram o documento** para quem não está logado (a LATAM devolve dois dígitos e o resto oculto). Documento mascarado é pior que documento nenhum — o blind index passaria a apontar para um valor que não é da pessoa —, então o mapeamento de cada provedor manda `nil` nesse caso, e `applyBooking` só sobrescreve quando veio algo de verdade.

## Os dois erros são o desenho inteiro

Toda falha de um provedor é uma de duas coisas, e elas significam o oposto uma da outra em todo lugar depois daqui:

| | O que é | Fallback? | Luz da cia |
|---|---|---|---|
| `ErrNotFound` | A aplicação da cia respondeu: essa reserva não existe | **só busca, nunca importa** | **verde** — respondeu |
| `ErrUnavailable` | Não chegamos na cia: borda, token recusado, timeout | **sim** | vermelha |

O mesmo teste separa os dois no fio: corpo JSON significa que o request chegou na aplicação da cia, HTML `Access Denied` da borda significa que não chegou.

**Depois de um `ErrNotFound`, a Traveos só pode *buscar* — nunca importar —, e isso guarda dinheiro de verdade.** A importação cria um bilhete real e não idempotente na vendor, e fazê-la para um localizador que a cia acabou de dizer que não existe é comprar bilhete para o nada.

A busca, por outro lado, é leitura, e é necessária: a cia direta identifica a reserva por mais que o localizador (a GOL exige origem e um sobrenome que batam), então o "não existe" dela muitas vezes é um erro de digitação na origem ou no sobrenome. A Traveos busca só pelo localizador e acha. Sem essa busca, ligar uma cia faria falhar emissões que funcionavam antes. Quando ninguém acha, o erro devolvido é o da cia, que diz ao operador o que conferir. Quem implementa isso é `issuance.attemptProviders` (`searchOnly`); os testes em `internal/issuance/sync_test.go` fixam as duas metades.

Use `Unavailable(cause, ...)` em vez de montar o erro à mão — ela **encadeia a causa** com `%w`. O handler de emissões responde alguns sentinelas do `carriers/traveos` com status próprio, e achatar a causa numa string rebaixaria todos eles em silêncio para o 502 genérico.

## `Importer` é para um provedor só

A Traveos não lê a cia sob demanda: a reserva precisa ser **importada** primeiro, para o scraper dela ir buscar, e só então pode ser lida. Nenhuma integração direta tem equivalente — ela pergunta para a própria cia.

Por isso `EnsureAvailable` é uma interface separada, e não um segundo método de `Provider`: porta cujo segundo método só uma implementação consegue honrar não é porta. No dia em que a última cia sair da vendor, essa interface e o único caller dela somem juntos.

Quem chama é responsável por só chegar lá **depois** de um `Fetch` ter voltado `ErrNotFound` — a emissão não é idempotente e não devolve id, então uma tentativa cega cria um segundo registro que ninguém distingue do primeiro.

## `Registry` não tem lock, e isso é uma afirmação

É montado uma vez em `cmd/api` e `cmd/bot` e só lido depois. **Os dois registram as mesmas cias diretas** por `bookingproviders.RegisterDirect` — até 23/09/2026 o bot registrava só a Traveos, e toda emissão vinda do WhatsApp ignorava `direct_booking_enabled`. Cia nova entra lá, não num `main.go`. Mapa que ninguém escreve depois do boot é seguro para leitura concorrente, e um mutex aqui sugeriria uma mutabilidade que não existe. `Watch` também é boot-time.

**Registrado e ligado são coisas diferentes.** `Registry.Register` diz que a integração existe; `airlines.direct_booking_enabled` diz que ela é usada. É isso que permite subir uma cia nova no escuro e ligá-la pelo painel depois, sem release no meio — ver `issuance.resolveProviders`.

## O provedor nunca é nil dentro de uma interface

`NewProvider` recebe o client concreto e quem constrói só chama dentro do `if client != nil`. Um `*traveos.Client` nil guardado numa interface **não é nil**, e `Registry.Empty()` acreditaria ter fallback. É a mesma armadilha que o `internal/bot` já teve que documentar uma vez.

## Onde mora o quê

| Camada | Pacote | Papel |
|---|---|---|
| Contrato | `internal/booking` | `Booking`, `Provider`, `Importer`, `Prober`, `Registry`, os dois sentinelas |
| Adaptadores | `carriers/traveos`, e um por cia (submódulo privado) | client puro + tradução para `Booking` |
| Uso | `internal/issuance` | escolhe quem responde, grava as linhas, registra `booking_source` |
| Saúde | `internal/health` | sonda sintética por provedor + resultado das buscas reais |

Ao acrescentar uma companhia: um pacote `carriers/<cia>` com o client puro (sem noção de HTTP de entrada, como `carriers/azul` e `carriers/smiles` já fazem), um `provider.go` que traduz para `Booking`, `testdata/` com um payload real anonimizado, e uma linha em `carriers/bookingproviders/register.go` (vale para a API e para o bot). Não mexa em `internal/issuance` — se precisou, o tipo neutro é que está faltando alguma coisa.

## Instrumentação temporária: o relatório de conexões diretas

Para decidir quando cada companhia pode deixar a Traveos, toda busca de itinerário grava **uma linha por provedor consultado** em `booking_lookup_attempts` (migration 66): companhia, provedor, resultado (`found`, `not_found`, `unavailable`, `refused`), erro resumido, duração e um `sync_id` que junta as tentativas da mesma sync. É o `sync_id` que diz "a direta falhou e a Traveos respondeu", o que `issuances.booking_source` sozinho não diz. A gravação é best-effort, no fim da `issuance.Sync`, e nunca derruba a sync. Vale para o painel e para o bot, porque os dois passam por ali.

A tela é **Dev Mode → CIAs Aéreas → Conexões diretas** (`/dashboard/airlines/direct-lookups`), servida por `GET /api/v1/booking-lookups/report?days=N`, só para dev. Uma companhia está pronta para deixar a Traveos quando "Caiu para a Traveos" fica em zero numa janela com volume. Com a conexão direta **desligada**, só a Traveos é consultada, então a tela não diz nada sobre a direta até ela ser ligada.

**Tudo isto sai junto com a Traveos.** Checklist de remoção:
- migration nova que faz `DROP TABLE booking_lookup_attempts`;
- `internal/data/booking_lookup_attempt.go`;
- em `internal/issuance/sync.go`: `lookupAttempt`, `recordLookupAttempts`, o parâmetro `record` de `attemptProviders` e o `syncID`;
- `internal/api/v1/{handlers/booking_lookup_report*.go,routes/booking_lookup*.go,schemas/booking_lookup.go}` e a linha em `v1/router.go`, com o Swagger regenerado;
- no painel: `src/app/dashboard/airlines/direct-lookups/`, `src/components/airlines/direct-lookups-table.tsx`, `src/schemas/booking-lookup.ts`, o botão em `dashboard/airlines/page.tsx`, a entrada em `lib/navigation.ts` e o rótulo em `nav-breadcrumb.tsx`.
