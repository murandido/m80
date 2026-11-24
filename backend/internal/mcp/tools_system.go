package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type noInput struct{}

func (s *Server) registerSystemTools(server *sdk.Server) {
	addTool(s, server, &sdk.Tool{
		Name:        "whoami",
		Description: "Mostra em nome de quem as ferramentas rodam: usuário, perfil (user/admin/dev) e agências visíveis. O estoque de pontos exige admin ou dev.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, _ noInput) (any, error) {
		var me any
		return me, s.get(ctx, who, "/auth/me", nil, &me)
	})

	addTool(s, server, &sdk.Tool{
		Name:        "system_status",
		Description: "Estado das dependências: sessão do bot de WhatsApp, Traveos e integrações das cias (ok/degradado/fora), com o horário da última checagem.",
		Annotations: readOnly(),
	}, func(ctx context.Context, who caller, _ noInput) (any, error) {
		var status any
		return status, s.get(ctx, who, "/status", nil, &status)
	})
}
