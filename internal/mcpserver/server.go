// Package mcpserver lets Claude Code in other projects drive Positron: an MCP server on a fixed
// localhost port, behind a bearer token kept in Positron's home folder.
package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GabiHert/positron/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DefaultPort is where the server listens; POSITRON_MCP_PORT overrides it.
const DefaultPort = "47615"

// Backend is what the tools call: every action of the app, plus sending and asking an agent.
type Backend interface {
	app.Backend
	// SendNow sends text to a chat at once, as the owner when agentID is "".
	SendNow(agentID, chatJID, text string) error
	// AskAgent has an agent do the owner's request in a chat and waits for the run (or ctx).
	AskAgent(ctx context.Context, agentID, chatJID, request string, review bool) (app.RunDTO, error)
	// ReplyFromClaude sends text as the owner, quoting quotedID when set. It never starts a run.
	ReplyFromClaude(ctx context.Context, chat, quotedID, text string) (app.ChatMessageDTO, error)
	// SendFileFromClaude sends a file as the owner. It never starts a run.
	SendFileFromClaude(ctx context.Context, in app.SendFileDTO) (app.ChatMessageDTO, error)
}

// Options configure a Server.
type Options struct {
	Home            string                   // the token lives at Home/mcp-token
	Port            string                   // "" = POSITRON_MCP_PORT, else DefaultPort
	DefaultRepoPath func(name string) string // where create_agent puts a new agent's repo
	WaitTimeout     time.Duration            // how long tools wait for Claude or an agent, default 5 minutes
}

// Server is Positron's MCP server for Claude Code in other projects. It is off until SetEnabled(true).
type Server struct {
	b    Backend
	opts Options

	mu    sync.Mutex
	token string
	srv   *http.Server
	err   string
}

// New returns a stopped server.
func New(b Backend, opts Options) *Server {
	if opts.Port == "" {
		opts.Port = os.Getenv("POSITRON_MCP_PORT")
	}
	if opts.Port == "" {
		opts.Port = DefaultPort
	}
	if opts.WaitTimeout <= 0 {
		opts.WaitTimeout = 5 * time.Minute
	}
	return &Server{b: b, opts: opts}
}

// URL is the address Claude Code connects to.
func (s *Server) URL() string { return "http://127.0.0.1:" + s.opts.Port + "/mcp" }

func (s *Server) tokenPath() string { return filepath.Join(s.opts.Home, "mcp-token") }

// Token returns the bearer token, creating it once.
func (s *Server) Token() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadToken()
}

func (s *Server) loadToken() (string, error) {
	if s.token != "" {
		return s.token, nil
	}
	if b, err := os.ReadFile(s.tokenPath()); err == nil {
		if t := strings.TrimSpace(string(b)); len(t) == 64 {
			s.token = t
			return t, nil
		}
	}
	return s.newToken()
}

func (s *Server) newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t := hex.EncodeToString(b)
	if err := os.MkdirAll(s.opts.Home, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(s.tokenPath(), []byte(t+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("could not save the token: %w", err)
	}
	_ = os.Chmod(s.tokenPath(), 0o600)
	s.token = t
	return t, nil
}

// RotateToken replaces the token. Claude Code has to be given the new one.
func (s *Server) RotateToken() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.newToken()
}

// Status says whether the server runs, and why not.
func (s *Server) Status() (running bool, errText string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.srv != nil, s.err
}

// SetEnabled starts or stops the server.
func (s *Server) SetEnabled(on bool) error {
	s.mu.Lock()
	if !on {
		srv := s.srv
		s.srv, s.err = nil, ""
		s.mu.Unlock()
		if srv != nil {
			// Requests in flight need the lock to check their token, so shut down without it.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if srv.Shutdown(ctx) != nil {
				_ = srv.Close()
			}
		}
		return nil
	}
	defer s.mu.Unlock()
	if s.srv != nil {
		return nil
	}
	if _, err := s.loadToken(); err != nil {
		s.err = err.Error()
		return err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+s.opts.Port)
	if err != nil {
		s.err = fmt.Sprintf("Port %s is in use, so the server could not start. Quit what uses it, or set POSITRON_MCP_PORT.", s.opts.Port)
		return errors.New(s.err)
	}
	s.err = ""
	s.srv = &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: s.Handler()}
	go func(srv *http.Server) { _ = srv.Serve(ln) }(s.srv)
	return nil
}

// Handler serves /mcp behind the Host, Origin and token checks.
func (s *Server) Handler() http.Handler {
	srv := s.mcpServer()
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	mux := http.NewServeMux()
	mux.Handle("/mcp", h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A web page must not reach this, even through DNS rebinding: only local hosts, no browser origins.
		if !localHost(r.Host) || r.Header.Get("Origin") != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if !s.authorized(r.Header.Get("Authorization")) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) authorized(header string) bool {
	got, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || got == "" {
		return false
	}
	want, err := s.Token()
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func localHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	return host == "127.0.0.1" || strings.EqualFold(host, "localhost")
}
