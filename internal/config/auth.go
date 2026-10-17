package config

import (
	"crypto/sha256"
	"fmt"
	"os"
)

// AccountToken is used for plex.tv discovery, never a server's resource token.
func (c *Config) AccountToken() string {
	if token := os.Getenv("PLEXCTL_TOKEN"); token != "" {
		return token
	}
	if c.HomeUser.AuthToken != "" {
		return c.HomeUser.AuthToken
	}
	return c.Token
}

// TokenSource binds discovered credentials to the account or Home profile that
// obtained them, without persisting a second copy of the account token.
func (c *Config) TokenSource() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(c.AccountToken())))
}

func (c *Config) HasServerToken() bool {
	_, server, ok := c.GetActiveServer()
	return ok && server.AccessToken != "" && server.TokenSource == c.TokenSource()
}

// ServerToken selects the same credentials for metadata, artwork, and streams.
func (c *Config) ServerToken() string {
	if c.HasServerToken() {
		return c.Servers[c.DefaultServer].AccessToken
	}
	if _, server, ok := c.GetActiveServer(); ok && server.Shared {
		// A changed account/profile must rediscover its scoped resource token.
		// Broad account credentials must never be sent to someone else's PMS.
		return ""
	}
	if token := os.Getenv("PLEXCTL_TOKEN"); token != "" {
		return token
	}
	if c.HomeUser.AccessToken != "" && (c.HomeUser.ServerID == "" || c.HomeUser.ServerID == c.DefaultServer) {
		return c.HomeUser.AccessToken
	}
	return c.AccountToken()
}

// MediaCacheNamespace prevents matching item IDs on different servers or
// libraries visible to another Plex profile from sharing cached results.
func (c *Config) MediaCacheNamespace() string {
	return c.ServerTarget().CacheNamespace()
}
