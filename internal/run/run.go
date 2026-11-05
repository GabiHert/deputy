// Package run wires Deputy together and opens the window.
package run

import (
	"context"
	"embed"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/GabiHert/deputy/internal/app"
	"github.com/GabiHert/deputy/internal/assistant"
	"github.com/GabiHert/deputy/internal/backend"
	"github.com/GabiHert/deputy/internal/claude"
	"github.com/GabiHert/deputy/internal/domain"
	"github.com/GabiHert/deputy/internal/engine"
	"github.com/GabiHert/deputy/internal/media"
	"github.com/GabiHert/deputy/internal/menubar"
	"github.com/GabiHert/deputy/internal/paths"
	"github.com/GabiHert/deputy/internal/speech"
	"github.com/GabiHert/deputy/internal/store"
	"github.com/GabiHert/deputy/internal/system"
	"github.com/GabiHert/deputy/internal/whatsapp"
	"github.com/google/uuid"
	wailsv2 "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Version is set at build time.
var Version = "0.1.0"

// cachedEnv reads the login shell environment once; it is slow and rarely changes.
func cachedEnv() func() []string {
	var once sync.Once
	var env []string
	return func() []string {
		once.Do(func() { env = claude.LoginShellEnv() })
		return append([]string(nil), env...)
	}
}

// Run starts the app.
func Run(assets embed.FS) error {
	if err := os.MkdirAll(paths.Home(), 0o700); err != nil {
		return err
	}
	if f, err := os.OpenFile(paths.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}
	st, err := store.Open(paths.DBPath())
	if err != nil {
		return err
	}
	st.FailRunning()
	wa, err := whatsapp.Open(context.Background(), paths.WhatsAppDBPath())
	if err != nil {
		return err
	}
	env := cachedEnv()
	gitEnv := func() []string { return claude.CleanEnv(env()) }
	go env() // warm the cache off the startup path

	var wctx context.Context
	var ctxMu sync.Mutex
	getCtx := func() context.Context {
		ctxMu.Lock()
		defer ctxMu.Unlock()
		return wctx
	}
	emit := func(event string, data any) {
		if c := getCtx(); c != nil {
			wruntime.EventsEmit(c, event, data)
		}
	}
	show := func() {
		if c := getCtx(); c != nil {
			wruntime.WindowShow(c)
			wruntime.WindowUnminimise(c)
		}
	}

	mediaStore := media.Store{Dir: paths.MediaDir()}
	engineSpeech := speech.New(speech.Config{ModelsDir: paths.ModelsDir()})
	detector := speech.NewDetector(paths.ModelsDir())
	models := speech.NewModelManager(paths.ModelsDir(), engineSpeech, detector)
	runner := claude.Runner{EnvFn: env}
	item := menubar.New()

	var be *backend.Backend
	notify := func(title, body string) {
		c := getCtx()
		if c == nil || !st.Settings().Notify {
			return
		}
		if err := wruntime.SendNotification(c, wruntime.NotificationOptions{ID: uuid.NewString(), Title: title, Body: body}); err != nil {
			log.Printf("notify: %v", err)
		}
	}
	eng := engine.New(engine.Deps{
		Store: st, WA: wa, Runner: runner, Media: mediaStore,
		Transcriber: media.Transcriber{Engine: engineSpeech, Detector: detector, TmpDir: os.TempDir()},
		Speaker:     media.Speaker{EnvFn: env, TmpDir: os.TempDir()},
		RepoEnv:     gitEnv,
		OnRun: func(r domain.Run) {
			if be == nil {
				return
			}
			emit(app.EventActivity, be.RunDTO(r))
			s := be.Status()
			emit(app.EventStatus, s)
			item.SetWaiting(s.Waiting)
		},
		OnChats: func() { emit(app.EventChats, nil) },
		Notify:  notify,
	})
	be = backend.New(backend.Deps{Store: st, WA: wa, Engine: eng, Media: mediaStore, Models: models, Runner: runner,
		Env: gitEnv, Emit: emit, Version: Version, Login: system.LoginItem{}})

	qe := be.QueryEngine()
	asst, err := assistant.New(assistant.Deps{
		Store: st, Query: qe, Env: env, RepoEnv: gitEnv, Dir: filepath.Join(paths.Home(), "assistant"), Emit: emit,
		OwnerName: func() string { return wa.State().PushName }, Board: be, DefaultRepoPath: backend.DefaultRepoPath,
	})
	if err != nil {
		return err
	}
	be.SetV2(asst, qe)
	scheduleCtx, stopSchedule := context.WithCancel(context.Background())
	go be.RunBoardSchedule(scheduleCtx)

	wa.OnState(func(whatsapp.State) { be.EmitStatus() })
	wa.OnMessage(eng.HandleIncoming)
	wa.OnGroups(func(gs []whatsapp.Group) {
		for _, g := range gs {
			c, err := st.Chat(g.JID)
			if err != nil {
				c = domain.Chat{JID: g.JID, IsGroup: true}
			}
			c.Name, c.Members, c.IsGroup = g.Name, g.Members, true
			_ = st.SaveChat(c)
		}
		emit(app.EventChats, nil)
	})

	bound, setCtx := app.New(be, func(ctx context.Context, title string) (string, error) {
		return wruntime.OpenDirectoryDialog(ctx, wruntime.OpenDialogOptions{Title: title, CanCreateDirectories: true})
	})

	go func() {
		// Media cleanup once a day.
		for {
			mediaStore.Prune(st.Settings().MediaRetentionDays)
			time.Sleep(24 * time.Hour)
		}
	}()

	return wailsv2.Run(&options.App{
		Title:             "Deputy",
		Width:             1280,
		Height:            820,
		MinWidth:          960,
		MinHeight:         620,
		BackgroundColour:  &options.RGBA{R: 251, G: 251, B: 248, A: 255},
		Mac:               &mac.Options{TitleBar: mac.TitleBarHiddenInset()},
		HideWindowOnClose: true,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               instanceID(),
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { show() },
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
			Handler: system.FileHandler{MediaDir: paths.MediaDir(), RepoDir: func(id string) (string, bool) {
				a, err := st.Agent(id)
				return a.RepoPath, err == nil
			}},
		},
		OnStartup: func(ctx context.Context) {
			ctxMu.Lock()
			wctx = ctx
			ctxMu.Unlock()
			setCtx(ctx)
			if err := wruntime.InitializeNotifications(ctx); err == nil {
				go func() { _, _ = wruntime.RequestNotificationAuthorization(ctx) }()
			} else {
				log.Printf("notifications: %v", err)
			}
			item.Install(menubar.Handlers{
				Activity: func() { show(); emit(app.EventNavigate, "activity") },
				Open:     show,
				Quit:     func() { wruntime.Quit(ctx) },
			})
			item.SetWaiting(be.Status().Waiting)
			go func() {
				if err := wa.Start(); err != nil {
					log.Printf("whatsapp: %v", err)
				}
			}()
		},
		OnShutdown: func(ctx context.Context) {
			stopSchedule()
			asst.Close()
			wa.Stop()
			eng.Close()
			engineSpeech.Unload()
			_ = st.Close()
		},
		Bind: []interface{}{bound},
	})
}

// instanceID is the single-instance lock. A separate DEPUTY_HOME is a separate Deputy, so it
// gets its own lock and can run next to the installed app.
func instanceID() string {
	if h := os.Getenv("DEPUTY_HOME"); h != "" {
		return "tech.bhlabs.deputy." + uuid.NewSHA1(uuid.NameSpaceURL, []byte(h)).String()[:8]
	}
	return "tech.bhlabs.deputy"
}
