// Package mcp serves the M80 to Claude as an MCP server (Streamable HTTP), authenticated by
// personal tokens (data.MCPToken). Every tool goes through the API's own router as the token
// owner - see loopback.
package mcp

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
)

const instructions = `M80 é o sistema de emissão de passagens com milhas da agência, em produção.
As ferramentas agem como o dono do token, com o mesmo papel e o mesmo escopo de agência que ele tem no painel.
Leituras respondem direto. Valores monetários em reais (decimal), pontos como inteiros, datas em AAAA-MM-DD.
Ids são UUID: descubra-os com as ferramentas de busca antes de pedir um registro.`

// NewHandler builds the /mcp endpoint. api is the API's top-level router: tools call back into
// it, so it must be the same router /mcp is mounted on - every middleware a panel request goes
// through, a tool call goes through too.
func NewHandler(db *gorm.DB, api http.Handler) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "m80", Version: "1"}, &mcp.ServerOptions{Instructions: instructions})
	addReadTools(server, loopback{api: api})

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		// Stateless with plain JSON answers: the panel relays each request as one fetch, so
		// there is no session to pin to a process and no SSE stream to keep open through Next.
		Stateless:    true,
		JSONResponse: true,
	})

	// Per token, on top of the router-wide limit the loopback calls already count against.
	// Generous for a person driving Claude, tight enough that a runaway loop stops quickly.
	limit := httprate.Limit(120, time.Minute, httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
		who, _ := callerFrom(r.Context())
		return "mcp:" + who.tokenID, nil
	}))

	return requireToken(db)(limit(handler))
}
