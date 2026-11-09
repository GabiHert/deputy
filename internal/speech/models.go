package speech

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/port"
)

var _ port.ModelManager = (*ModelManager)(nil)

// ErrDownloadRunning is returned when a download starts while another runs.
var ErrDownloadRunning = errors.New("A model download is already running.")

// legacyWhisperSmallDirName is the folder earlier builds installed the Portuguese
// model into. Nothing reads it now that Portuguese uses turbo.
const legacyWhisperSmallDirName = "sherpa-onnx-whisper-small"

// ModelManager reports which speech models are installed and downloads the rest.
type ModelManager struct {
	modelsDir string
	engine    *Engine
	detector  *Detector

	mu          sync.Mutex
	downloading map[domain.Language]bool
	running     bool
	onReady     func(domain.Language)
}

// NewModelManager builds a manager over the engine and detector it keeps ready.
func NewModelManager(modelsDir string, engine *Engine, detector *Detector) *ModelManager {
	return &ModelManager{modelsDir: modelsDir, engine: engine, detector: detector, downloading: map[domain.Language]bool{}}
}

// SetOnReady registers fn, called after a successful Download.
func (m *ModelManager) SetOnReady(fn func(domain.Language)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onReady = fn
}

// installed reports whether the artifact's folder (or raw file) is on disk.
func (m *ModelManager) installed(a ModelArtifact) bool {
	p := filepath.Join(m.modelsDir, a.Dir)
	if a.File != "" {
		p = filepath.Join(p, a.File)
	}
	_, err := os.Stat(p)
	return err == nil
}

// Status reports whether lang is ready and what a download would still fetch.
func (m *ModelManager) Status(lang domain.Language) port.ModelStatus {
	st := port.ModelStatus{Language: lang}
	m.mu.Lock()
	st.Downloading = m.downloading[lang]
	m.mu.Unlock()

	engineOK, engineMsg := m.engine.ReadyFor(lang)
	detOK, detMsg := m.detector.Ready()
	st.Ready = engineOK && detOK
	switch {
	case !engineOK:
		st.Missing = engineMsg
	case !detOK:
		st.Missing = detMsg
	}
	manifest, err := loadManifest()
	if err != nil {
		st.Ready = false
		st.Missing = err.Error()
		return st
	}
	for _, a := range manifest.forLanguage(lang) {
		if !m.installed(a) {
			st.DownloadBytes += a.Size
		}
	}
	return st
}

// Download installs every missing artifact for lang, one download at a time.
func (m *ModelManager) Download(ctx context.Context, lang domain.Language, onProgress func(port.ModelProgress)) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return ErrDownloadRunning
	}
	m.running = true
	m.downloading[lang] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.running = false
		m.downloading[lang] = false
		m.mu.Unlock()
	}()

	fail := func(name string, err error) error {
		onProgress(port.ModelProgress{Language: lang, Name: name, Phase: "error", Err: err.Error()})
		return err
	}
	manifest, err := loadManifest()
	if err != nil {
		return fail("", err)
	}
	for _, a := range manifest.forLanguage(lang) {
		if m.installed(a) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return fail(a.Name, err)
		}
		report := func(phase string, done, total int64) {
			onProgress(port.ModelProgress{Language: lang, Name: a.Name, Phase: phase, Done: done, Total: total})
		}
		if err := downloadAndExtract(ctx, m.modelsDir, a, report); err != nil {
			return fail(a.Name, err)
		}
	}
	m.removeLegacyPT(manifest, lang)
	onProgress(port.ModelProgress{Language: lang, Phase: "done"})

	m.mu.Lock()
	cb := m.onReady
	m.mu.Unlock()
	if cb != nil {
		cb(lang)
	}
	return nil
}

// removeLegacyPT removes the old Portuguese model folder after a successful
// Portuguese download. It does nothing for another language, while turbo is not
// ready, or when the active manifest installs a Portuguese artifact into that
// folder. A failure is only logged: the download itself succeeded.
func (m *ModelManager) removeLegacyPT(manifest *DownloadManifest, lang domain.Language) {
	if !isPTBR(lang) {
		return
	}
	if ok, _ := m.engine.ReadyFor(domain.LangPTBR); !ok {
		return
	}
	for _, a := range manifest.forLanguage(domain.LangPTBR) {
		if a.Dir == legacyWhisperSmallDirName {
			return
		}
	}
	if err := os.RemoveAll(filepath.Join(m.modelsDir, legacyWhisperSmallDirName)); err != nil {
		log.Printf("remove legacy speech model: %v", err)
	}
}
