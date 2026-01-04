package relay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"webhookrelay/internal/config"
)

type ForwarderConfig struct {
	Logger         *slog.Logger
	Concurrency    int
	ForwardTimeout time.Duration
}

type Forwarder struct {
	log     *slog.Logger
	client  *http.Client
	sem     chan struct{}
	timeout time.Duration
}

func NewForwarder(cfg ForwarderConfig) *Forwarder {
	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 50
	}
	if cfg.ForwardTimeout <= 0 {
		cfg.ForwardTimeout = 10 * time.Second
	}

	return &Forwarder{
		log:     log,
		client:  &http.Client{},
		sem:     make(chan struct{}, cfg.Concurrency),
		timeout: cfg.ForwardTimeout,
	}
}

func (f *Forwarder) ForwardAsync(ctx context.Context, reqID string, relayName string, relayID string, inbound *http.Request, body []byte, destinations []config.DestinationConfig) {
	// We intentionally do not wait. Each destination forward runs in its own goroutine.
	for _, d := range destinations {
		dest := d
		go f.forwardOne(ctx, reqID, relayName, relayID, inbound, body, dest)
	}
}

func (f *Forwarder) forwardOne(parentCtx context.Context, reqID string, relayName string, relayID string, inbound *http.Request, body []byte, dest config.DestinationConfig) {
	select {
	case f.sem <- struct{}{}:
		defer func() { <-f.sem }()
	case <-parentCtx.Done():
		return
	}

	start := time.Now()

	method := inbound.Method
	if dest.Method != "" {
		method = dest.Method
	}

	ctx, cancel := context.WithTimeout(parentCtx, f.timeout)
	defer cancel()

	outReq, err := http.NewRequestWithContext(ctx, method, dest.URL, bytes.NewReader(body))
	if err != nil {
		f.log.Error("forward: build request failed", "request_id", reqID, "relay", relayName, "dest_url", dest.URL, "error", err)
		return
	}

	copyHeaders(outReq.Header, inbound.Header)
	outReq.Host = ""
	outReq.Header.Del("Host")
	applyHeaderOverrides(outReq.Header, dest.Headers)

	// Loop prevention / trace propagation:
	// - Each relay appends its relay id to X-WebhookRelay-Trace.
	// - This lets any relay detect that it's seeing a relayed request it already processed.
	relayID = strings.TrimSpace(relayID)
	if relayID != "" {
		outReq.Header.Set(HeaderTrace, appendTrace(outReq.Header.Get(HeaderTrace), relayID))
	}
	outReq.Header.Set(HeaderRequestID, reqID)

	resp, err := f.client.Do(outReq)
	latencyMS := time.Since(start).Milliseconds()
	if err != nil {
		// Distinguish timeouts/cancel for better logs.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			f.log.Warn("forward: timeout", "request_id", reqID, "relay", relayName, "dest_url", dest.URL, "latency_ms", latencyMS, "error", err)
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			f.log.Warn("forward: canceled", "request_id", reqID, "relay", relayName, "dest_url", dest.URL, "latency_ms", latencyMS, "error", err)
			return
		}
		f.log.Error("forward: request failed", "request_id", reqID, "relay", relayName, "dest_url", dest.URL, "latency_ms", latencyMS, "error", err)
		return
	}
	_ = resp.Body.Close()

	f.log.Info("forward: completed", "request_id", reqID, "relay", relayName, "dest_url", dest.URL, "status", resp.StatusCode, "latency_ms", latencyMS)
}

func copyHeaders(dst http.Header, src http.Header) {
	for k, vv := range src {
		ck := http.CanonicalHeaderKey(k)
		if isHopByHopHeader(ck) {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func applyHeaderOverrides(h http.Header, overrides map[string]string) {
	for k, v := range overrides {
		if strings.EqualFold(k, "host") {
			continue
		}
		h.Set(k, v)
	}
}

func isHopByHopHeader(k string) bool {
	switch http.CanonicalHeaderKey(k) {
	case "Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade":
		return true
	default:
		return false
	}
}

const (
	HeaderTrace     = "X-WebhookRelay-Trace"
	HeaderRequestID = "X-WebhookRelay-Request-Id"
)

func appendTrace(existing string, instanceID string) string {
	existing = strings.TrimSpace(existing)
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return existing
	}
	if existing == "" {
		return instanceID
	}

	parts := strings.Split(existing, ",")
	out := make([]string, 0, len(parts)+1)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	out = append(out, instanceID)
	return strings.Join(out, ",")
}
