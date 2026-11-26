package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
)

type Request struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations"`

	Run func(ctx context.Context, args json.RawMessage) (any, error) `json:"-"`
}

type Server struct {
	Name         string
	Version      string
	Instructions string
	Tools        []Tool
}

var versions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

var (
	ReadOnly    = map[string]any{"readOnlyHint": true}
	CreateOnly  = map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false}
	Destructive = map[string]any{"readOnlyHint": false, "destructiveHint": true}
)

func (s *Server) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		var req Request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			enc.Encode(Response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &Error{-32700, "parse error"}})
			continue
		}
		if resp := s.Handle(ctx, req); resp != nil {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

func (s *Server) Handle(ctx context.Context, req Request) *Response {
	if len(req.ID) == 0 {
		return nil
	}
	resp := &Response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &p)
		version := versions[0]
		for _, v := range versions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		result := map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
		}
		if s.Instructions != "" {
			result["instructions"] = s.Instructions
		}
		resp.Result = result
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": s.Tools}
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = &Error{-32602, "bad params"}
			break
		}
		t, ok := s.find(p.Name)
		if !ok {
			resp.Error = &Error{-32602, "unknown tool: " + p.Name}
			break
		}
		if len(p.Arguments) == 0 {
			p.Arguments = json.RawMessage("{}")
		}
		out, err := t.Run(ctx, p.Arguments)
		if err != nil {
			resp.Result = map[string]any{"content": []any{text(err.Error())}, "isError": true}
			break
		}
		b, _ := json.Marshal(out)
		resp.Result = map[string]any{"content": []any{text(string(b))}, "structuredContent": out}
	default:
		resp.Error = &Error{-32601, "method not found: " + req.Method}
	}
	return resp
}

func (s *Server) find(name string) (Tool, bool) {
	for _, t := range s.Tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

func text(s string) map[string]any {
	return map[string]any{"type": "text", "text": s}
}

func Schema(props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

func Decode(args json.RawMessage, v any) error {
	d := json.NewDecoder(bytes.NewReader(args))
	d.UseNumber()
	return d.Decode(v)
}
