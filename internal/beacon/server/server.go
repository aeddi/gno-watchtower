// Package server wraps a Noise listener with an HTTP server that
// reverse-proxies every request to the configured upstream. Bearer tokens and
// all other request headers travel through unmodified — the beacon is a dumb
// pipe at this layer. Augmentation of /rpc payloads happens at a higher
// layer (see internal/beacon/augment) via an optional hook.
package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aeddi/gno-watchtower/pkg/noise"
)

// BodyTransform is an optional per-path hook called before the request body
// is forwarded upstream. Returns the possibly-modified body. Implementations
// MUST be safe against malformed input — they shouldn't panic — and MUST NOT
// leak the bearer token or other sensitive headers.
//
// path is the incoming URL path (e.g. "/rpc"). body is the full request body
// (already buffered into memory). When nil is returned, forward the original.
type BodyTransform func(ctx context.Context, path string, body []byte) []byte

// maxBeaconBodyBytes bounds the sentinel→beacon request body we'll buffer
// before forwarding upstream. Matches the watchtower's own
// handlers.maxBodyBytes (50 MiB) so a body that passes the beacon also passes
// the watchtower, and neither side buffers more than the other accepts.
const maxBeaconBodyBytes = 50 << 20

// Server is the beacon's Noise-listening HTTP server.
type Server struct {
	upstream   *url.URL
	noiseCfg   *noise.Config
	listenAddr string
	handshakeT time.Duration
	transform  BodyTransform // optional; called per incoming request

	log      *slog.Logger
	lis      *noise.Listen
	srv      *http.Server
	upClient *http.Client // upstream HTTP client — configured, never http.DefaultClient
}

// Config collects the parameters Server needs.
type Config struct {
	ListenAddr       string
	UpstreamURL      string
	NoiseConfig      *noise.Config
	HandshakeTimeout time.Duration
	Transform        BodyTransform // nil → pure passthrough
	Log              *slog.Logger
}

// New builds a Server. Listening + serving begin when Run is called.
func New(cfg Config) (*Server, error) {
	if cfg.UpstreamURL == "" {
		return nil, fmt.Errorf("upstream URL is required")
	}
	u, err := url.Parse(cfg.UpstreamURL)
	if err != nil {
		return nil, fmt.Errorf("parse upstream URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("upstream URL must be http:// or https://, got %q", u.Scheme)
	}
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		upstream:   u,
		noiseCfg:   cfg.NoiseConfig,
		listenAddr: cfg.ListenAddr,
		handshakeT: cfg.HandshakeTimeout,
		transform:  cfg.Transform,
		log:        log.With("component", "beacon_server"),
		// A dedicated client so we don't inherit http.DefaultClient's
		// (lack of) timeout or redirect policy. Redirects are refused
		// outright — preserving Authorization across a 3xx to another
		// host would leak the bearer token.
		upClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// rejectLogLevel classifies a handshake rejection error. EOF (both io.EOF
// and io.ErrUnexpectedEOF) is common and benign after a beacon restart —
// the running sentinel's first retry arrives before the noise handshake
// completes. Everything else (bad keys, wrong protocol) still gets WARN.
func rejectLogLevel(err error) slog.Level {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return slog.LevelDebug
	}
	return slog.LevelWarn
}

// Run starts the Noise listener + HTTP server and blocks until ctx is
// cancelled, then performs a graceful shutdown with a short deadline.
func (s *Server) Run(ctx context.Context) error {
	lis, err := noise.NewListener("tcp", s.listenAddr, *s.noiseCfg, s.handshakeT,
		func(remote net.Addr, err error) {
			s.log.Log(ctx, rejectLogLevel(err), "handshake rejected", "remote", remote, "err", err)
		})
	if err != nil {
		return fmt.Errorf("noise listen: %w", err)
	}
	s.lis = lis

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	s.srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Shutdown goroutine: when ctx is cancelled, drain the server with a
	// bounded deadline so a stuck upstream doesn't block forever.
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutCtx)
	}()

	s.log.Info("beacon listening", "addr", s.listenAddr, "upstream", s.upstream.String())
	if err := s.srv.Serve(s.lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// handle is the single HTTP handler installed on the Noise listener. It
// reverse-proxies every request to the upstream unchanged (optionally
// running the body through Transform first).
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBeaconBodyBytes)
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		s.log.Warn("read request body", "path", r.URL.Path, "err", err)
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	// Optional transform — augmenter hooks here. If it panics or returns nil,
	// fall through to the original body; never drop a request because of an
	// augmentation issue.
	if s.transform != nil {
		out := safeTransform(r.Context(), s.log, s.transform, r.URL.Path, body)
		if out != nil {
			body = out
		}
	}

	upURL := *s.upstream
	// Preserve query string; the upstream watchtower may use it.
	upURL.Path = singleJoin(upURL.Path, r.URL.Path)
	upURL.RawQuery = r.URL.RawQuery

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upURL.String(), bytes.NewReader(body))
	if err != nil {
		s.log.Error("build upstream request", "err", err)
		http.Error(w, "upstream", http.StatusInternalServerError)
		return
	}
	// Copy all headers from the inbound request — Authorization, Content-Type,
	// Content-Encoding, User-Agent, etc. Remove hop-by-hop ones per RFC 7230.
	copyHeaders(req.Header, r.Header)

	resp, err := s.upClient.Do(req)
	if err != nil {
		s.log.Error("forward to upstream", "path", r.URL.Path, "err", err)
		http.Error(w, "upstream unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// safeTransform calls fn but recovers from panics, logging and returning nil
// so the request falls back to the original body. Augmentation is
// best-effort: a crash in the hook must not drop the sentinel's data.
func safeTransform(ctx context.Context, log *slog.Logger, fn BodyTransform, path string, body []byte) (out []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("transform panic", "path", path, "err", r)
			out = nil
		}
	}()
	return fn(ctx, path, body)
}

// hopByHop headers per RFC 7230 §6.1 — must not be forwarded by intermediaries.
var hopByHop = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		if _, hop := hopByHop[http.CanonicalHeaderKey(k)]; hop {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

// singleJoin concatenates two URL path components ensuring exactly one slash
// at the boundary.
func singleJoin(a, b string) string {
	switch {
	case strings.HasSuffix(a, "/") && strings.HasPrefix(b, "/"):
		return a + b[1:]
	case strings.HasSuffix(a, "/") || strings.HasPrefix(b, "/"):
		return a + b
	default:
		return a + "/" + b
	}
}
