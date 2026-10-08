package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
	"github.com/google/jsonschema-go/jsonschema"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// handlerFunc is a tool body. It reads its arguments through the ToolRequest
// and answers with a JSON-object text result (handlers.ToolSuccess) or an error
// result (handlers.ToolError).
type handlerFunc func(context.Context, *handlers.ToolRequest) (*sdkmcp.CallToolResult, error)

// toolSpec is the metadata of one tool; the input schema comes from the type
// parameter of addTool and the output schema from output.
type toolSpec struct {
	name        string
	title       string
	description string
	annotations *sdkmcp.ToolAnnotations
	// output is a zero value of the struct documenting structuredContent.
	output any
	// enums lists the allowed values of string properties the Go type cannot express.
	enums map[string][]string
	// readsConfig makes the tool fail like the CLI when the project's
	// configuration does not load, instead of listing nothing.
	readsConfig bool
}

// addTool registers a tool through the SDK's typed AddTool: In is the argument
// struct (the SDK infers and enforces its schema, rejecting unknown or mistyped
// arguments), and the result carries structuredContent validated against the
// output schema next to the text the handler produced. An error result keeps
// IsError, so a failure is never reported as success.
func addTool[In any](s *Server, spec toolSpec, handler handlerFunc) {
	in, err := inputSchemaFor[In](spec.enums)
	if err != nil {
		panic(fmt.Sprintf("tool %s: input schema: %v", spec.name, err))
	}
	out, err := outputSchemaFor(spec.output)
	if err != nil {
		panic(fmt.Sprintf("tool %s: output schema: %v", spec.name, err))
	}
	annotations := *spec.annotations
	annotations.Title = spec.title
	tool := &sdkmcp.Tool{
		Name:         spec.name,
		Title:        spec.title,
		Description:  spec.description,
		Annotations:  &annotations,
		InputSchema:  in,
		OutputSchema: out,
	}
	_, takesDir := in.Properties[argWorkingDirectory]
	if spec.readsConfig {
		handler = handlers.WithLoadableConfig(handler)
	}
	sdkmcp.AddTool(s.mcpServer, tool, func(ctx context.Context, req *sdkmcp.CallToolRequest, _ In) (*sdkmcp.CallToolResult, map[string]any, error) {
		args, err := rawArguments(req)
		if err != nil {
			return nil, nil, err
		}
		if takesDir {
			if err := s.dirs.confine(args); err != nil {
				return nil, nil, err
			}
		}
		wrapper := handlers.NewToolRequest(req, args)
		res, err := handler(ctx, wrapper)
		if err != nil {
			return nil, nil, err
		}
		doc, text := resultDocument(res)
		if res.IsError {
			if doc == nil {
				return nil, nil, errors.New(text)
			}
			return res, doc, nil
		}
		s.emitToolTelemetry(ctx, tool.Name, wrapper)
		if doc == nil {
			doc = map[string]any{"result": text}
		}
		return res, doc, nil
	})
}

// rawArguments decodes the call's arguments, which the SDK already validated
// against the input schema.
func rawArguments(req *sdkmcp.CallToolRequest) (map[string]any, error) {
	args := map[string]any{}
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return args, nil
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("decoding arguments: %w", err)
	}
	return args, nil
}

// resultDocument returns the JSON object in the first text block of res (nil
// when that text is not an object) and that text.
func resultDocument(res *sdkmcp.CallToolResult) (doc map[string]any, text string) {
	if res == nil {
		return nil, ""
	}
	for _, c := range res.Content {
		t, ok := c.(*sdkmcp.TextContent)
		if !ok {
			continue
		}
		text = t.Text
		dec := json.NewDecoder(bytes.NewReader([]byte(t.Text)))
		var parsed map[string]any
		if err := dec.Decode(&parsed); err == nil && parsed != nil {
			doc = parsed
		}
		return doc, text
	}
	return nil, ""
}

// inputSchemaFor infers the argument schema of In and applies the enums.
func inputSchemaFor[In any](enums map[string][]string) (*jsonschema.Schema, error) {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		return nil, fmt.Errorf("infer: %w", err)
	}
	for name, values := range enums {
		prop, ok := schema.Properties[name]
		if !ok {
			return nil, fmt.Errorf("enum for unknown property %q", name)
		}
		prop.Enum = make([]any, len(values))
		for i, v := range values {
			prop.Enum[i] = v
		}
	}
	return schema, nil
}

// outputSchemaFor infers the schema of the result struct and relaxes it: no
// property is required, containers may be null (an empty list can serialize as
// null) and unlisted properties are allowed, so a result may gain a field
// without failing validation.
func outputSchemaFor(shape any) (*jsonschema.Schema, error) {
	schema, err := jsonschema.ForType(reflect.TypeOf(shape), nil)
	if err != nil {
		return nil, fmt.Errorf("infer: %w", err)
	}
	schema.Required = nil
	schema.AdditionalProperties = nil
	for _, prop := range schema.Properties {
		if prop.Type == "array" || prop.Type == "object" {
			prop.Types, prop.Type = []string{prop.Type, "null"}, ""
		}
	}
	return schema, nil
}
