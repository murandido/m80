package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/datatypes"
)

// toolFunc is a tool body: it gets who is calling and the decoded input, and returns anything
// JSON-encodable, which is handed to the agent as is.
type toolFunc[In any] func(ctx context.Context, who caller, in In) (any, error)

// addTool registers a tool whose every call - success or failure - is recorded in mcp_calls.
// That row is the only trail of what the agent wrote without asking: audit_logs covers
// issuances alone, and records no edits at all.
func addTool[In any](s *Server, server *sdk.Server, tool *sdk.Tool, fn toolFunc[In]) {
	sdk.AddTool(server, tool, func(ctx context.Context, req *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
		who, err := callerFrom(req)
		if err != nil {
			return nil, nil, err
		}

		started := s.now()
		outcome := &callOutcome{}
		out, callErr := fn(context.WithValue(ctx, outcomeKey{}, outcome), who, in)
		s.recordCall(who, tool.Name, in, started, outcome, callErr)
		if callErr != nil {
			return nil, nil, callErr
		}

		text, err := json.Marshal(out)
		if err != nil {
			return nil, nil, err
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(text)}}}, nil, nil
	})
}

func (s *Server) recordCall(who caller, tool string, in any, started time.Time, outcome *callOutcome, callErr error) {
	args, err := json.Marshal(in)
	if err != nil {
		args = []byte(`null`)
	}
	// A write that became a proposal is marked in its args: the trail shows it, and the
	// daily batch ceiling does not count it as a batch written.
	if outcome != nil && outcome.Proposed {
		var fields map[string]any
		if json.Unmarshal(args, &fields) == nil && fields != nil {
			fields["_outcome"] = "proposed"
			if marked, err := json.Marshal(fields); err == nil {
				args = marked
			}
		}
	}
	row := data.MCPCall{
		TokenID:    who.TokenID,
		Tool:       tool,
		Args:       datatypes.JSON(args),
		OK:         callErr == nil,
		DurationMs: int(s.now().Sub(started).Milliseconds()),
	}
	if callErr != nil {
		message := callErr.Error()
		row.Error = &message
	}
	// A failed trail write must not fail the call: the write it describes has already
	// happened, and reporting it as failed would invite the agent to repeat it.
	if err := s.db.Create(&row).Error; err != nil {
		slog.Error("mcp: failed to record tool call", "tool", tool, "token_id", who.TokenID, "err", err)
	}
}

// readOnly annotates a tool for the client, which may use it to decide what to
// confirm. It is a hint only: what actually holds the agent back is the proposals flow.
func readOnly() *sdk.ToolAnnotations {
	return &sdk.ToolAnnotations{ReadOnlyHint: true}
}
