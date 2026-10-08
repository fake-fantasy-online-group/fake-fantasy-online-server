package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// The local build script injects this path. FANTASY_PROJECT_DIR takes precedence,
// and packaged builds also scan their parent directories for server/go.mod.
var builtRepoRoot string

//go:embed assets/controller.png
var controllerIcon []byte

func main() {
	application := app.NewWithID("org.fakefantasy.server-controller")
	application.SetIcon(fyne.NewStaticResource("controller.png", controllerIcon))
	root, err := locateRepository()
	if err != nil {
		window := application.NewWindow("Fantasy服务端控制器")
		window.SetContent(container.NewPadded(widget.NewLabel("定位项目失败：\n" + err.Error())))
		window.ShowAndRun()
		return
	}

	controller := newController(root)
	controller.logf("controller", "success", "控制器已启动")
	newDesktopUI(application, controller).showAndRun()
}

func locateRepository() (string, error) {
	candidates := []string{os.Getenv("FANTASY_PROJECT_DIR"), builtRepoRoot}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(executable))
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		candidate, _ = filepath.Abs(candidate)
		for i := 0; i < 8; i++ {
			if repositoryAt(candidate) {
				return filepath.Clean(candidate), nil
			}
			parent := filepath.Dir(candidate)
			if parent == candidate {
				break
			}
			candidate = parent
		}
	}
	return "", fmt.Errorf("未找到 server/go.mod；请把控制器放在项目目录中，或设置 FANTASY_PROJECT_DIR")
}

func repositoryAt(path string) bool {
	data, err := os.ReadFile(filepath.Join(path, "server", "go.mod"))
	return err == nil && strings.Contains(string(data), "module github.com/fake-fantasy-online-group/fake-fantasy-online-server/server")
}

func commandContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}
