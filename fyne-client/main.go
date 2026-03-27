package main

import (
	"fmt"
	"fyne-client/proxy"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ─── Custom Dark Theme ───────────────────────────────────────────────────────

type myTheme struct{}

func (m myTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0x0f, G: 0x17, B: 0x2a, A: 0xff}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 0x1e, G: 0x29, B: 0x3b, A: 0xff}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0x38, G: 0xbd, B: 0xf8, A: 0xff}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0xf8, G: 0xfa, B: 0xfc, A: 0xff}
	case theme.ColorNameButton:
		return color.NRGBA{R: 0x1e, G: 0x29, B: 0x3b, A: 0xff}
	}
	return theme.DefaultTheme().Color(n, v)
}

func (m myTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
func (m myTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (m myTheme) Size(n fyne.ThemeSizeName) float32       { return theme.DefaultTheme().Size(n) }

// ─── Main ────────────────────────────────────────────────────────────────────

func main() {
	a := app.New()
	a.Settings().SetTheme(&myTheme{})
	w := a.NewWindow("VK-TURN Proxy")
	w.Resize(fyne.NewSize(480, 560))

	p := proxy.NewProxy()

	// ── Status indicator ──
	grayCol := color.NRGBA{0x64, 0x74, 0x8b, 0xff}
	dot := canvas.NewRectangle(grayCol)
	dotWrap := container.NewWithoutLayout(dot)
	dot.Resize(fyne.NewSize(10, 10))
	dotWrap.Resize(fyne.NewSize(10, 10))

	statusLabel := canvas.NewText("Idle", grayCol)
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	statusBar := container.NewHBox(dotWrap, statusLabel)

	setStatus := func(label string, col color.Color) {
		statusLabel.Text = label
		statusLabel.Color = col
		statusLabel.Refresh()
		dot.FillColor = col
		dot.Refresh()
	}

	// ── Inputs (Tunnel) ──
	linkEntry := widget.NewEntry()
	linkEntry.SetPlaceHolder("https://vk.com/call/join/...")

	peerEntry := widget.NewEntry()
	peerEntry.SetPlaceHolder("193.124.224.119:56000")

	listenEntry := widget.NewEntry()
	listenEntry.SetText("127.0.0.1:443")

	// ── Inputs (WireGuard mode) ──
	wgConfEntry := widget.NewMultiLineEntry()
	wgConfEntry.SetPlaceHolder("[Interface]\nPrivateKey = ...\nAddress = ...\nDNS = ...\n\n[Peer]\nPublicKey = ...\nPresharedKey = ...\nAllowedIPs = ...\nEndpoint = ...")
	wgConfEntry.Wrapping = fyne.TextWrapOff

	// ── Buttons ──
	var startBtn, stopBtn *widget.Button

	startBtn = widget.NewButtonWithIcon("CONNECT", theme.MediaPlayIcon(), func() {
		l := linkEntry.Text
		pa := peerEntry.Text
		la := listenEntry.Text
		if l == "" || pa == "" {
			dialog.ShowError(fmt.Errorf("заполните Link и Peer"), w)
			return
		}
		getCreds, cleanLink := parseLink(l)
		params := &proxy.TurnParams{
			Link:     cleanLink,
			Udp:      false,
			GetCreds: getCreds,
		}
		setStatus("Соединение...", color.NRGBA{0x38, 0xbd, 0xf8, 0xff})
		startBtn.Disable()
		stopBtn.Enable()
		go func() {
			wgCfg := strings.TrimSpace(wgConfEntry.Text)
			if wgCfg == "" {
				setStatus("Ошибка: вставьте WireGuard .conf", color.NRGBA{0xef, 0x44, 0x44, 0xff})
				startBtn.Enable()
				stopBtn.Disable()
				return
			}
			if err := p.StartWithVPN(params, la, pa, 8, false, wgCfg); err != nil {
				setStatus("Ошибка: "+err.Error(), color.NRGBA{0xef, 0x44, 0x44, 0xff})
				startBtn.Enable()
				stopBtn.Disable()
				return
			}
			setStatus("Активен ✓", color.NRGBA{0x22, 0xc5, 0x5e, 0xff})
		}()
	})
	startBtn.Importance = widget.HighImportance

	stopBtn = widget.NewButtonWithIcon("DISCONNECT", theme.MediaStopIcon(), func() {
		p.Stop()
		setStatus("Idle", grayCol)
		startBtn.Enable()
		stopBtn.Disable()
	})
	stopBtn.Importance = widget.DangerImportance
	stopBtn.Disable()

	// ── Instructions ──
	steps := widget.NewRichTextFromMarkdown(`**Как подключиться:**

1. На VPS поднять WireGuard + сервер (см. README.md)
2. Вставить ссылку VK/Yandex и адрес VPS → **CONNECT**
3. Вставить WireGuard .conf и нажать CONNECT`)
	steps.Wrapping = fyne.TextWrapWord

	// ── Title ──
	title := canvas.NewText("VK-TURN Proxy", color.NRGBA{0x38, 0xbd, 0xf8, 0xff})
	title.TextSize = 22
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	// ── Layout ──
	commonForm := widget.NewForm(
		widget.NewFormItem("Link", linkEntry),
		widget.NewFormItem("Peer (VPS)", peerEntry),
		widget.NewFormItem("Listen", listenEntry),
	)

	content := container.NewVBox(
		container.NewPadded(title),
		widget.NewCard("Туннель (TURN/DTLS)", "", commonForm),
		widget.NewCard("WireGuard конфиг (.conf)", "", container.NewMax(wgConfEntry)),
		container.NewGridWithColumns(2, startBtn, stopBtn),
		widget.NewCard("Инструкция", "", steps),
		widget.NewCard("", "", statusBar),
	)

	w.SetContent(container.NewPadded(content))
	w.SetMaster()
	w.ShowAndRun()
}

func parseLink(link string) (proxy.GetCredsFunc, string) {
	var getCreds proxy.GetCredsFunc
	var clean string
	if strings.Contains(link, "vk.com/call/join/") {
		parts := strings.Split(link, "join/")
		clean = parts[len(parts)-1]
		getCreds = proxy.GetVkCreds
	} else if strings.Contains(link, "telemost.yandex.ru/j/") {
		parts := strings.Split(link, "j/")
		clean = parts[len(parts)-1]
		getCreds = proxy.GetYandexCreds
	} else {
		clean = link
		getCreds = proxy.GetVkCreds
	}
	if idx := strings.IndexAny(clean, "/?#"); idx != -1 {
		clean = clean[:idx]
	}
	return getCreds, clean
}
