package system

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// FileHandler serves chat media under /media/ and agents' repo files under /repo/<agent id>/.
type FileHandler struct {
	MediaDir string
	RepoDir  func(agentID string) (string, bool)
}

func serveInside(w http.ResponseWriter, r *http.Request, root, rel string) {
	root, _ = filepath.Abs(root)
	p := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	if p != root && !strings.HasPrefix(p, root+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, p)
}

func (h FileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path, err := url.PathUnescape(r.URL.EscapedPath())
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasPrefix(path, "/media/"):
		serveInside(w, r, h.MediaDir, strings.TrimPrefix(path, "/media/"))
	case strings.HasPrefix(path, "/repo/") && h.RepoDir != nil:
		rest := strings.TrimPrefix(path, "/repo/")
		id, rel, ok := strings.Cut(rest, "/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		dir, found := h.RepoDir(id)
		if !found || strings.HasPrefix(rel, ".git/") {
			http.NotFound(w, r)
			return
		}
		serveInside(w, r, dir, rel)
	default:
		http.NotFound(w, r)
	}
}
