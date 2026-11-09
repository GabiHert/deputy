# Notices

Positron copies code from Notted, which copies and modifies code from quant (github.com/brienze1/quant), under the MIT License
below. Each copied file starts with a header naming its quant source, and is listed in the table.
Positron never imports quant's packages.

## quant license

MIT License

Copyright (c) 2026 Luis Brienze

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## Copied files

| Notted file | quant source | Changes |
|---|---|---|
| `internal/infra/paths/paths.go` | `internal/infra/paths/paths.go` (lines 1-28) | renamed to NOTTED_HOME and ~/.notted; added DBPath, AudioDir, ModelsDir |
| `Makefile` | `Makefile` (lines 32-33, 87-88, 107-108) | kept dev/build/test only; added check and frontend gates; no release, brew or network targets |
| `internal/infra/store/store.go` | `internal/infra/db/sqlite.go` (lines 18-44) | path parameter, 0700 folder, busy timeout, one connection, versioned migrations |
| `internal/integration/audio/wavinfo/decode.go` | `internal/integration/voice/engine/wav.go` (lines 1-123) | package renamed; also exposes the fmt fields |
| `internal/integration/speech/sherpa.go` | `internal/integration/voice/engine/sherpaengine/sherpaengine.go` (lines 1-97, 124-132, 199-292, 354-381, 472-555) | speech-to-text only; Kokoro removed; language per call; decodes with wavinfo.DecodeWAV |
| `internal/integration/speech/manifest.go` | `internal/integration/voiceruntime/manifest.go` (lines 13-110) | NOTTED_MODEL_MANIFEST; per-language selection; raw-file artifacts |
| `internal/integration/speech/download.go` | `internal/integration/voiceruntime/download.go` (lines 1-237) | folders as parameters; raw-file install |
| `internal/integration/claude/env.go` | `internal/application/service/job_manager.go` (lines 1827-1848) | renamed LoginShellEnv, now reads the interactive login shell (marker-parsed, 10s timeout, plain-login and os.Environ fallbacks) and prepends the folder holding claude when PATH lacks it; added CleanEnv and LookPath |
| `internal/integration/claude/runner.go` | `internal/application/service/job_manager.go` (lines 844-982) | fixed read-only argument lists; no shell; no stream-json; credentials removed; never the permission-skipping flag |
| `internal/integration/process/manager.go` | `internal/integration/process/manager.go` (lines 85-115, 245-494, 551-562) | generic argv from the caller; no shell, persona, MCP, output file or remote; Wails events per meeting; no automatic respawn; the permission-skipping flag removed |
| `internal/application/service/instructions_fs.go` | `internal/application/service/file_service.go` (lines 70-104, 198-238, 311-332) | rooted at the archive folder; instruction patterns only; no events |
| `frontend/src/components/MarkdownView.tsx` | `frontend/src/components/MarkdownView.tsx` (lines 1-32) | no Mermaid, no highlighting; Ledger notes style |
| `frontend/src/components/TerminalPane.tsx` | `frontend/src/components/TerminalPane.tsx` (lines 1-4, 36-145, 425-470, 590-645) | per-meeting events; direct bindings; Ledger colours; no replay, voice or remote |
| `scripts/fixup-macos-dylibs.sh` | `scripts/fixup-macos-dylibs.sh` (lines 1-154) | Notted paths |
| `frontend/src/components/CodeEditor.tsx` | `frontend/src/components/CodeEditor.tsx` (lines 1-77) | Markdown only; Ledger theme from CSS variables |
| `frontend/src/components/FileEditor.tsx` | `frontend/src/components/FileTabPanel.tsx` (lines 15-21 and the text-file save flow) | instruction files only; no images, HTML or tabs |

## Code from Notted

Notted is by the same author as Positron. These Positron files adapt Notted's self-update code:

| Positron file | Notted source | Changes |
|---|---|---|
| `internal/update/update.go` | `internal/application/service/update.go` | downloads a GitHub tarball instead of pulling a clone; build has network access; deletes the code after the build |
| `internal/update/install.go`, `internal/update/install.sh` | `internal/integration/install/install.go`, `install.sh` | POSITRON_* overrides; deletes the staged app after copying it; no permission reset (Positron asks for none) |
| `internal/update/stream.go` | `internal/integration/process/stream.go` | a function instead of a type; uses claude.LookPath |

## Fonts

Newsreader and IBM Plex Sans are bundled under the SIL Open Font License 1.1; see
`frontend/src/assets/fonts/OFL-*.txt`. The woff2 files come from the npm packages
`@fontsource-variable/newsreader` 5.3.0 (Newsreader, optical-size variable files, latin and
latin-ext) and `@fontsource/ibm-plex-sans` 5.3.0 (IBM Plex Sans 400, 500 and 600, latin and
latin-ext), fetched with `npm pack`. The packages are not dependencies, and nothing is fetched
at runtime.
