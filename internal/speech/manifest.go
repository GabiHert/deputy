// Copied from github.com/brienze1/quant (internal/integration/voiceruntime/manifest.go), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.

package speech

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GabiHert/positron/internal/domain"
)

// defaultManifestJSON is the manifest pinned to this build: the upstream
// sherpa-onnx model files with verified sha256 and sizes. A local or hosted
// manifest can be substituted at runtime via DEPUTY_MODEL_MANIFEST (an
// http(s):// URL or a filesystem path), which tests use to point at local copies.
//
//go:embed manifest.json
var defaultManifestJSON []byte

// ModelArtifact is one downloadable model: a .tar.bz2/.tar.gz archive, or a raw file.
type ModelArtifact struct {
	// Name is the short human label streamed in progress events.
	Name string `json:"name"`
	// URL is an http(s):// URL, or a local filesystem path (tests).
	URL string `json:"url"`
	// SHA256 is the file digest, lowercase hex; verified after download.
	SHA256 string `json:"sha256"`
	// Size is the byte size; drives the download progress and the status total.
	Size int64 `json:"size"`
	// Dir is the folder under the models dir the artifact installs to.
	Dir string `json:"dir"`
	// File, when set, marks a raw file installed as Dir/File with no extraction.
	File string `json:"file,omitempty"`
	// Lang is the language the artifact serves; empty means every language.
	Lang string `json:"lang,omitempty"`
}

// neededFor reports whether the artifact is needed to serve lang.
func (a ModelArtifact) neededFor(lang domain.Language) bool {
	return a.Lang == "" || a.Lang == string(lang)
}

// DownloadManifest is the small, version-pinned index of model artifacts.
type DownloadManifest struct {
	Schema  int             `json:"schema"`
	Version string          `json:"version"`
	Models  []ModelArtifact `json:"models"`
}

// forLanguage returns the artifacts needed for lang, in manifest order.
func (m *DownloadManifest) forLanguage(lang domain.Language) []ModelArtifact {
	var out []ModelArtifact
	for _, a := range m.Models {
		if a.neededFor(lang) {
			out = append(out, a)
		}
	}
	return out
}

// loadManifest resolves the download manifest, honoring the
// DEPUTY_MODEL_MANIFEST override (http(s):// or a local path) and falling
// back to the embedded default.
func loadManifest() (*DownloadManifest, error) {
	raw := defaultManifestJSON
	if override := strings.TrimSpace(os.Getenv("DEPUTY_MODEL_MANIFEST")); override != "" {
		b, err := readSource(override)
		if err != nil {
			return nil, fmt.Errorf("load model manifest from %q: %w", override, err)
		}
		raw = b
	}
	var m DownloadManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse model manifest: %w", err)
	}
	return &m, nil
}

// readSource fetches bytes from an http(s):// URL or a local filesystem path.
func readSource(src string) ([]byte, error) {
	if isHTTP(src) {
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(src)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(src)
}

func isHTTP(src string) bool {
	return strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://")
}
