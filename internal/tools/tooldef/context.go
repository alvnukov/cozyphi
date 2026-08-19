package tooldef

import (
	"context"
	"strings"
)

type toolCallIDKey struct{}

type cwdKey struct{}

// WithToolCallID attaches the active tool_use id to ctx for UI correlation.
func WithToolCallID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, toolCallIDKey{}, id)
}

// ToolCallID returns the tool_use id from ctx, or empty.
func ToolCallID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(toolCallIDKey{}).(string)
	return v
}

// WithCwd attaches the session working directory used to resolve relative tool paths.
// Empty cwd is ignored so callers can pass through an unset session cwd.
func WithCwd(ctx context.Context, cwd string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return ctx
	}
	return context.WithValue(ctx, cwdKey{}, cwd)
}

func cwdFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(cwdKey{}).(string)
	return strings.TrimSpace(v)
}
