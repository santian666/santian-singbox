package main

import (
	"embed"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

const appTitle = "SanTian-Singbox"
const quitExistingArg = "--quit-existing"
const defaultWindowWidth = 1200
const defaultWindowHeight = 800
const minWindowWidth = 980
const minWindowHeight = 620

func main() {
	if handled, exitCode := runSystemProxyRASHelperFromArgs(); handled {
		os.Exit(exitCode)
	}
	if handled, exitCode := runExternalProxyProbeFromArgs(); handled {
		os.Exit(exitCode)
	}
	if handled, exitCode := runProxyWatchdogFromArgs(); handled {
		os.Exit(exitCode)
	}
	if hasArg(quitExistingArg) {
		if err := signalExistingInstanceToQuit(); err != nil {
			println("Quit signal failed:", err.Error())
		}
		return
	}
	if provider, err := detectIncompatibleWinsockProvider(); err == nil && provider != "" {
		showIncompatibleWinsockProvider(provider)
		return
	}
	locked := false
	if !isWailsDevMode() {
		ok, err := acquireSingleInstanceLock()
		if err != nil {
			println("Startup failed:", err.Error())
			return
		}
		if !ok {
			if isRunAsAdmin() {
				if err := quitExistingInstanceForAdminStart(8 * time.Second); err == nil {
					ok, err = acquireSingleInstanceLock()
					if err == nil && ok {
						locked = true
					}
				}
			}
			if !locked {
				activateRunningInstance()
				println("Application is already running")
				return
			}
		} else {
			locked = true
		}
		if locked {
			defer releaseSingleInstanceLock()
		}
	}

	app := NewApp()

	err := wails.Run(&options.App{
		Title:     appTitle,
		Width:     defaultWindowWidth,
		Height:    defaultWindowHeight,
		MinWidth:  minWindowWidth,
		MinHeight: minWindowHeight,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 244, G: 247, B: 251, A: 1},
		Windows: &windows.Options{
			Theme:                windows.Light,
			WebviewGpuIsDisabled: true,
			OnSuspend:            app.onSystemSuspend,
			OnResume:             app.onSystemResume,
			CustomTheme: &windows.ThemeSettings{
				DarkModeTitleBar:           windows.RGB(18, 18, 24),
				DarkModeTitleBarInactive:   windows.RGB(24, 24, 31),
				DarkModeTitleText:          windows.RGB(245, 247, 250),
				DarkModeTitleTextInactive:  windows.RGB(188, 194, 205),
				DarkModeBorder:             windows.RGB(47, 34, 70),
				DarkModeBorderInactive:     windows.RGB(34, 34, 44),
				LightModeTitleBar:          windows.RGB(244, 247, 251),
				LightModeTitleBarInactive:  windows.RGB(236, 241, 247),
				LightModeTitleText:         windows.RGB(15, 23, 42),
				LightModeTitleTextInactive: windows.RGB(100, 116, 139),
				LightModeBorder:            windows.RGB(214, 222, 235),
				LightModeBorderInactive:    windows.RGB(226, 232, 240),
			},
		},
		OnStartup:     app.startup,
		OnShutdown:    app.shutdown,
		OnBeforeClose: app.beforeClose,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

func quitExistingInstanceForAdminStart(timeout time.Duration) error {
	if err := signalExistingInstanceToQuit(); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ok, err := acquireSingleInstanceLock()
		if err != nil {
			return err
		}
		if ok {
			releaseSingleInstanceLock()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return os.ErrDeadlineExceeded
}

func isWailsDevMode() bool {
	return os.Getenv("devserver") != "" || os.Getenv("frontenddevserverurl") != ""
}

func hasArg(name string) bool {
	for _, arg := range os.Args[1:] {
		if arg == name {
			return true
		}
	}
	return false
}
