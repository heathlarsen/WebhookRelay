package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"path"
	"strings"
)

// ResolvedRelay is the runtime representation of a relay with a concrete listen path.
type ResolvedRelay struct {
	ID           string
	Name         string
	ListenPath   string
	Methods      []string
	Destinations []DestinationConfig
}

func ResolveRelays(cfg Config) ([]ResolvedRelay, error) {
	res := make([]ResolvedRelay, 0, len(cfg.Relays))
	for i, r := range cfg.Relays {
		lp := strings.TrimSpace(r.ListenPath)
		if lp == "" {
			tok, err := randomToken(16)
			if err != nil {
				return nil, fmt.Errorf("generate listen_path for relays[%d]: %w", i, err)
			}
			lp = joinPaths(cfg.Server.BasePath, "/"+tok)
		} else {
			lp = joinPaths(cfg.Server.BasePath, lp)
		}

		res = append(res, ResolvedRelay{
			ID:           relayID(lp),
			Name:         r.Name,
			ListenPath:   lp,
			Methods:      append([]string(nil), r.Methods...),
			Destinations: append([]DestinationConfig(nil), r.Destinations...),
		})
	}
	return res, nil
}

func relayID(listenPath string) string {
	// Stable for the lifetime of the process (because resolved listen paths are stable).
	// Hash keeps the trace header compact and avoids leaking the listen path.
	sum := sha256.Sum256([]byte(listenPath))
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	// 16 chars is plenty for collision resistance in a small routing table.
	return strings.ToLower(enc.EncodeToString(sum[:]))[:16]
}

func joinPaths(basePath, listenPath string) string {
	basePath = strings.TrimSpace(basePath)
	listenPath = strings.TrimSpace(listenPath)

	if basePath == "" {
		if listenPath == "" {
			return "/"
		}
		if !strings.HasPrefix(listenPath, "/") {
			listenPath = "/" + listenPath
		}
		return path.Clean(listenPath)
	}

	if listenPath == "" {
		return path.Clean(basePath)
	}
	if !strings.HasPrefix(listenPath, "/") {
		listenPath = "/" + listenPath
	}
	return path.Clean(basePath + listenPath)
}

func randomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	// Lower-case for nicer URLs.
	return strings.ToLower(enc.EncodeToString(b)), nil
}


