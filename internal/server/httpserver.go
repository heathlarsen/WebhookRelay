package server

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"webhookrelay/internal/config"
)

type Forwarder interface {
	ForwardAsync(ctx context.Context, reqID string, relayName string, inbound *http.Request, body []byte, destinations []config.DestinationConfig)
}

type Config struct {
	Logger     *slog.Logger
	ListenAddr string
	Relays     []config.ResolvedRelay
	Forwarder  Forwarder
}

type Server struct {
	log *slog.Logger
	srv *http.Server
}

func New(cfg Config) *Server {
	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	mux := http.NewServeMux()

	// Health endpoint for convenience.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	for _, r := range cfg.Relays {
		relay := r
		p := cleanPath(relay.ListenPath)
		mux.HandleFunc(p, func(w http.ResponseWriter, req *http.Request) {
			handleRelay(log, cfg.Forwarder, relay, w, req)
		})
	}

	return &Server{
		log: log,
		srv: &http.Server{
			Addr:    cfg.ListenAddr,
			Handler: mux,
		},
	}
}

func (s *Server) Run() error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.srv.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		s.log.Info("shutdown signal received", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.srv.Shutdown(ctx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func handleRelay(log *slog.Logger, fwd Forwarder, relay config.ResolvedRelay, w http.ResponseWriter, req *http.Request) {
	if !methodAllowed(req.Method, relay.Methods) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Read the entire body so we can fan-out to multiple destinations.
	body, err := io.ReadAll(req.Body)
	if err != nil {
		log.Error("read body failed", "relay", relay.Name, "path", relay.ListenPath, "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_ = req.Body.Close()

	reqID, _ := newRequestID()

	// Fire-and-forget forwarding. We do NOT tie it to req.Context() because that
	// context is canceled when the handler returns.
	if fwd != nil {
		fwd.ForwardAsync(context.Background(), reqID, relay.Name, req, body, relay.Destinations)
	}

	w.Header().Set("X-Relay-Request-Id", reqID)
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("accepted"))
}

func methodAllowed(method string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	m := strings.ToUpper(strings.TrimSpace(method))
	for _, a := range allowed {
		if m == strings.ToUpper(strings.TrimSpace(a)) {
			return true
		}
	}
	return false
}

func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = path.Clean(p)
	if p == "." {
		return "/"
	}
	return p
}

func newRequestID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	return fmt.Sprintf("%s", strings.ToLower(enc.EncodeToString(b[:]))), nil
}


