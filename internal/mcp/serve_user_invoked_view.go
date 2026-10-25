package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
)

// hidesUserInvoked reports whether this server withholds user-only skills from
// the model: a serving server (skills-only or merged) that was not told to serve
// them. The plain authoring server (`ai-rulez mcp` without --serve-skills) has no
// serving state; it edits the configuration, so it reads and lists every skill.
func (s *Server) hidesUserInvoked() bool {
	return s.serve != nil && !s.serve.opts.ServeUserInvoked
}

// refuseUserInvokedPath is the error a read of a discovered skill by its source
// path gets on a server that hides user-only skills, or nil.
func (s *Server) refuseUserInvokedPath(ref string) error {
	if s.hidesUserInvoked() && s.cat().UserInvokedPath(ref) {
		return userInvokedError(strings.TrimSpace(ref))
	}
	return nil
}

// guardUserInvokedRead wraps the authoring read_skill so it refuses a skill
// whose SKILL.md sets disable-model-invocation, whatever it is called.
func (s *Server) guardUserInvokedRead(next handlerFunc) handlerFunc {
	return func(ctx context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
		name := req.GetString(keyName, "")
		if err := s.refuseUserInvokedPath(name); err != nil {
			return handlers.ToolError(err)
		}
		res, err := next(ctx, req)
		if err != nil || res == nil || res.IsError || !s.hidesUserInvoked() {
			return res, err
		}
		if doc, _ := resultDocument(res); doc != nil {
			if content, ok := doc["content"].(string); ok && contentUserInvoked(content) {
				return handlers.ToolError(userInvokedError(name))
			}
		}
		return res, nil
	}
}

// hideUserInvokedList wraps list_skills so it leaves out the skills whose
// SKILL.md sets disable-model-invocation.
func (s *Server) hideUserInvokedList(next handlerFunc) handlerFunc {
	return func(ctx context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
		res, err := next(ctx, req)
		if err != nil || res == nil || res.IsError || !s.hidesUserInvoked() {
			return res, err
		}
		doc, _ := resultDocument(res)
		list, ok := doc["skills"].([]any)
		if !ok {
			return res, nil
		}
		kept := make([]any, 0, len(list))
		for _, e := range list {
			if m, isMap := e.(map[string]any); isMap && skillFileUserInvoked(m["path"]) {
				continue
			}
			kept = append(kept, e)
		}
		if len(kept) == len(list) {
			return res, nil
		}
		doc["skills"], doc[keyCount] = kept, len(kept)
		return handlers.ToolSuccess(doc)
	}
}

// skillFileUserInvoked reports whether the SKILL.md at the given path sets
// disable-model-invocation; an unreadable file is not held.
func skillFileUserInvoked(p any) bool {
	file, ok := p.(string)
	if !ok {
		return false
	}
	content, err := os.ReadFile(file) //nolint:gosec // a path the authoring tool just listed
	return err == nil && contentUserInvoked(string(content))
}

// keepExistingPathQuiet wraps create_skill so that, on a server that hides
// user-only skills, the "already exists" error names the skill and not the file
// it lives in. That a name is taken stays visible: an author needs it.
func (s *Server) keepExistingPathQuiet(next handlerFunc) handlerFunc {
	return func(ctx context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
		res, err := next(ctx, req)
		if err != nil || res == nil || !res.IsError || !s.hidesUserInvoked() {
			return res, err
		}
		if strings.Contains(firstText(res), " already exists at ") {
			return handlers.ToolError(fmt.Errorf("skill %q already exists", req.GetString(keyName, "")))
		}
		return res, nil
	}
}
