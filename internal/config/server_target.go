package config

import (
	"context"
	"crypto/sha256"
	"fmt"
)

// ServerTarget is a snapshot of a connection. Views and playback retain it so
// visiting a friend's library never changes the configured default server.
type ServerTarget struct {
	ID           string
	Name         string
	URL          string
	Token        string `json:"-"`
	AccountToken string `json:"-"`
	Shared       bool
}

func (c *Config) ServerTarget() ServerTarget {
	id, server, _ := c.GetActiveServer()
	return ServerTarget{
		ID: id, Name: server.Name, URL: server.URL, Token: c.ServerToken(),
		AccountToken: c.AccountToken(), Shared: server.Shared,
	}
}

// MediaServer supplies the current server for callers without an explicit target.
func MediaServer(targets ...ServerTarget) ServerTarget {
	if len(targets) > 0 && targets[0].ID != "" {
		return targets[0]
	}
	return Get().ServerTarget()
}

func (s ServerTarget) CacheNamespace() string {
	return fmt.Sprintf("%s/%x", s.ID, sha256.Sum256([]byte(s.URL+"\x00"+s.Token)))
}

type serverContextKey struct{}

func (s ServerTarget) Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, serverContextKey{}, s)
}

func ServerFromContext(ctx context.Context) ServerTarget {
	if server, ok := ctx.Value(serverContextKey{}).(ServerTarget); ok {
		return server
	}
	return Get().ServerTarget()
}
