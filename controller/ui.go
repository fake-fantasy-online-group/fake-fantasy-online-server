package main

import (
	"fmt"
	"image/color"
	"net/url"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type serviceRow struct {
	dot      *canvas.Circle
	detail   *widget.Label
	endpoint *widget.Label
	button   *widget.Button
	phase    string
}

type desktopUI struct {
	app        fyne.App
	window     fyne.Window
	controller *controller

	overall    *widget.Label
	repository *widget.Label
	startAll   *widget.Button
	stopAll    *widget.Button
	openWeb    *widget.Button
	rows       map[serviceID]*serviceRow
	operation  atomic.Bool
}

func newDesktopUI(application fyne.App, controller *controller) *desktopUI {
	ui := &desktopUI{
		app: application, window: application.NewWindow("服务端控制器"), controller: controller,
		rows: make(map[serviceID]*serviceRow),
	}
	ui.build()
	return ui
}

func (ui *desktopUI) build() {
	title := widget.NewLabelWithStyle("服务端控制器", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	ui.repository = widget.NewLabel(ui.controller.root)
	ui.repository.TextStyle = fyne.TextStyle{Monospace: true}
	ui.repository.Truncation = fyne.TextTruncateEllipsis
	ui.overall = widget.NewLabel("正在检查…")
	header := container.NewBorder(nil, nil, nil, ui.overall, container.NewVBox(title, ui.repository))

	ui.startAll = widget.NewButton("全部启动", func() {
		ui.runOperation("正在启动全部服务…", ui.controller.startAll, nil)
	})
	ui.startAll.Importance = widget.HighImportance
	ui.stopAll = widget.NewButton("全部关闭", func() {
		ui.runOperation("正在关闭全部服务…", ui.controller.stopAll, nil)
	})
	ui.stopAll.Importance = widget.DangerImportance
	ui.openWeb = widget.NewButton("打开数据库网页", func() {
		ui.runOperation("正在准备数据库网页…", ui.controller.prepareDatabaseWeb, func() {
			address, _ := url.Parse("http://127.0.0.1:8088/fantasy-db/")
			if err := ui.app.OpenURL(address); err != nil {
				dialog.ShowError(err, ui.window)
				return
			}
			ui.controller.logf("controller", "success", "已打开数据库网页")
		})
	})
	actions := container.NewHBox(ui.startAll, ui.stopAll, ui.openWeb)

	serviceObjects := []fyne.CanvasObject{}
	for index, service := range []serviceID{serviceDatabase, serviceDispatch, serviceGame} {
		row := ui.newServiceRow(service)
		ui.rows[service] = row
		serviceObjects = append(serviceObjects, ui.serviceObject(service, row))
		if index < 2 {
			serviceObjects = append(serviceObjects, widget.NewSeparator())
		}
	}
	services := container.NewPadded(container.NewVBox(serviceObjects...))

	footer := widget.NewLabel("关闭数据库不会删除数据。关闭窗口不会自动停服。")
	footer.Alignment = fyne.TextAlignCenter

	content := container.NewVBox(header, widget.NewSeparator(), actions, widget.NewSeparator(), services, footer)
	ui.window.SetContent(container.NewPadded(content))
	ui.window.Resize(fyne.NewSize(720, 360))
	ui.window.SetMaster()
}

func (ui *desktopUI) newServiceRow(service serviceID) *serviceRow {
	button := widget.NewButton("启动", func() {
		row := ui.rows[service]
		if row != nil && row.phase == "running" {
			ui.runOperation("正在关闭"+displayName(service)+"…", func() error { return ui.controller.stopService(service) }, nil)
			return
		}
		ui.runOperation("正在启动"+displayName(service)+"…", func() error { return ui.controller.startService(service) }, nil)
	})
	return &serviceRow{
		dot:    canvas.NewCircle(color.NRGBA{R: 142, G: 142, B: 147, A: 255}),
		detail: widget.NewLabel("正在检查…"), endpoint: widget.NewLabel(""), button: button,
	}
}

func (ui *desktopUI) serviceObject(service serviceID, row *serviceRow) fyne.CanvasObject {
	name := widget.NewLabelWithStyle(displayName(service), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	row.detail.Truncation = fyne.TextTruncateEllipsis
	row.endpoint.TextStyle = fyne.TextStyle{Monospace: true}
	row.endpoint.Alignment = fyne.TextAlignTrailing
	right := container.NewHBox(container.NewGridWrap(fyne.NewSize(160, 30), row.endpoint), container.NewGridWrap(fyne.NewSize(82, 36), row.button))
	center := container.NewVBox(name, row.detail)
	return container.NewBorder(nil, nil, container.NewGridWrap(fyne.NewSize(14, 14), row.dot), right, center)
}

func (ui *desktopUI) showAndRun() {
	go ui.refreshLoop()
	ui.window.ShowAndRun()
}

func (ui *desktopUI) refreshLoop() {
	for {
		snapshot := ui.controller.snapshot()
		fyne.Do(func() { ui.applySnapshot(snapshot) })
		time.Sleep(2 * time.Second)
	}
}

func (ui *desktopUI) applySnapshot(snapshot statusResponse) {
	if snapshot.AllReady {
		ui.overall.SetText("全部运行中")
	} else {
		running := 0
		for _, status := range snapshot.Services {
			if status.Phase == "running" {
				running++
			}
		}
		ui.overall.SetText(fmt.Sprintf("%d / 3 运行中", running))
	}
	for _, status := range snapshot.Services {
		row := ui.rows[status.ID]
		if row == nil {
			continue
		}
		row.phase = status.Phase
		row.detail.SetText(status.Detail)
		row.endpoint.SetText(status.Endpoint)
		row.button.SetText("启动")
		row.button.Importance = widget.HighImportance
		row.dot.FillColor = color.NRGBA{R: 142, G: 142, B: 147, A: 255}
		switch status.Phase {
		case "running":
			row.button.SetText("关闭")
			row.button.Importance = widget.DangerImportance
			row.dot.FillColor = theme.Color(theme.ColorNameSuccess)
		case "starting":
			row.dot.FillColor = theme.Color(theme.ColorNameWarning)
		case "error", "conflict":
			row.dot.FillColor = theme.Color(theme.ColorNameError)
		}
		row.dot.Refresh()
		row.button.Refresh()
	}
	ui.setButtonsEnabled(!ui.operation.Load() && snapshot.Busy == "")
}

func (ui *desktopUI) runOperation(label string, operation func() error, success func()) {
	if !ui.operation.CompareAndSwap(false, true) {
		return
	}
	ui.controller.setBusy(label)
	ui.overall.SetText(label)
	ui.setButtonsEnabled(false)
	go func() {
		ui.controller.operationMu.Lock()
		err := operation()
		ui.controller.operationMu.Unlock()
		ui.controller.setBusy("")
		ui.operation.Store(false)
		snapshot := ui.controller.snapshot()
		fyne.Do(func() {
			ui.applySnapshot(snapshot)
			if err != nil {
				dialog.ShowError(err, ui.window)
				return
			}
			if success != nil {
				success()
			}
		})
	}()
}

func (ui *desktopUI) setButtonsEnabled(enabled bool) {
	buttons := []*widget.Button{ui.startAll, ui.stopAll, ui.openWeb}
	for _, row := range ui.rows {
		buttons = append(buttons, row.button)
	}
	for _, button := range buttons {
		if enabled {
			button.Enable()
		} else {
			button.Disable()
		}
	}
}
