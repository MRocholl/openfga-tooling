package agent

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServeMCP answers agent queries about the workspace over stdio.
func ServeMCP(ctx context.Context, w *Workspace, version string) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "fga", Version: version}, nil)

	register(server, w)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("mcp server: %w", err)
	}

	return nil
}

type checkInput struct {
	File string `json:"file,omitempty" jsonschema:"limit the check to one file, relative to the workspace root; omit to check everything"`
}

type nameInput struct {
	Name string `json:"name" jsonschema:"a type like 'document', or a relation like 'document#can_view'"`
}

type typeInput struct {
	Type string `json:"type" jsonschema:"the type name, for example 'document'"`
}

type queryInput struct {
	Query string `json:"query" jsonschema:"text to match against type, relation and condition names; matches loosely, so 'cvw' finds 'can_view'"`
}

func register(server *mcp.Server, w *Workspace) {
	text := func(s string) *mcp.CallToolResult {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
	}

	mcp.AddTool(server, &mcp.Tool{
		Name: "fga_check",
		Description: "Validate the OpenFGA models and store tests. Reports the same errors as " +
			"`fga model validate`, with the offending line and a caret under it. Start here when " +
			"asked whether a model is correct.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in checkInput) (*mcp.CallToolResult, any, error) {
		w.Reload()

		return text(w.Check(in.File)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "fga_overview",
		Description: "List the modules, types and conditions in the workspace, with relation counts. " +
			"Use this first when unfamiliar with a model.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		w.Reload()

		return text(w.Overview()), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "fga_describe",
		Description: "Show a type's relations, merged across every module that declares or extends it, " +
			"with the definition of each.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in typeInput) (*mcp.CallToolResult, any, error) {
		return text(w.Describe(in.Type)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fga_definition",
		Description: "Show where a type or relation is defined, with the surrounding lines.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in nameInput) (*mcp.CallToolResult, any, error) {
		return text(w.Definition(in.Name)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "fga_references",
		Description: "Find every use of a type or relation, in models and in .fga.yaml store tests. " +
			"Use before changing or removing one.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in nameInput) (*mcp.CallToolResult, any, error) {
		return text(w.References(in.Name)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fga_search",
		Description: "Find type, relation and condition names matching a query.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in queryInput) (*mcp.CallToolResult, any, error) {
		return text(w.Search(in.Query)), nil, nil
	})
}
