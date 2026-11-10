package model

import (
	"context"
	"encoding/json"
)

// Tool is an action Core or a Module offers to Connectors over MCP, such as
// searching Words. Core serves every Tool to every Connector without knowing
// what any of them does.
type Tool struct {
	// Name is unique among all Tools. A Module's start with the Module's
	// name, such as japanese_search.
	Name string
	// Description tells the Connector's model what the Tool does and when to
	// use it.
	Description string
	// InputSchema is the JSON Schema of the Tool's arguments, of type object.
	InputSchema json.RawMessage
	// ReadOnly is true if the Tool changes nothing.
	ReadOnly bool
	// Destructive is true if the Tool deletes, merges or moves data, so the
	// Connector can be set to ask the Owner before each use.
	Destructive bool
	// Handle runs the Tool with its arguments, as JSON, and returns its
	// result, which the Connector gets as JSON: best a JSON object. An error
	// is shown to the Connector's model as the Tool's failure.
	Handle func(ctx context.Context, arguments json.RawMessage) (any, error)
}

// ToolProvider is what Core or a Module offers Connectors. Each one with
// Tools adds its provider once, at startup, before serving.
type ToolProvider interface {
	Tools() []Tool
}
