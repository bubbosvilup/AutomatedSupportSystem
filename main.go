//go:build windows

// A.S.S. — Automated Support System.
// Genera il messaggio "dispositivo pronto per il ritiro" da copiare e incollare
// su WhatsApp Business. Interfaccia in stile Windows 98 disegnata a mano.
// Win32 puro tramite syscall: nessuna dipendenza esterna, un solo .exe portable.
package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

const (
	appShort   = "A.S.S."
	appName    = "Automated Support System"
	appTitle   = appShort + " - " + appName
	appVersion = "1.1.0"
)

// Messaggio predefinito. Si può sovrascrivere mettendo un file "messaggio.txt"
// accanto all'exe, con gli stessi segnaposto.
const defaultTemplate = `[SALUTO] [NOME],

ti informiamo che il tuo [DISPOSITIVO] è pronto per il ritiro presso il nostro negozio.

Puoi passare a ritirarlo durante i nostri consueti orari di apertura, consultabili online e su questo profilo WhatsApp.

Per qualsiasi informazione o necessità, siamo a tua disposizione.

Grazie per aver scelto *FlashinLabs*!

*Il team di FlashinLabs*`

var devices = []string{
	"PC", "PC fisso", "notebook", "PC gaming", "Mac", "MacBook",
	"smartphone", "iPhone", "tablet", "iPad", "console", "monitor", "stampante",
}

const (
	idCopy   = 1 // IDOK: Invio da tastiera equivale a "Copia"
	idName   = 101
	idDevice = 102
	idPrev   = 103
	idNew    = 105
	idMin    = 201
	idClose  = 202

	timerStatus = 1
	timerClock  = 2

	wmApp = 0x8000
)

// Palette Windows 98 (COLORREF = 0x00BBGGRR).
func rgb(r, g, b uint32) uint32 { return r | g<<8 | b<<16 }

var (
	cWhite  = rgb(255, 255, 255)
	cLight  = rgb(223, 223, 223)
	cFace   = rgb(192, 192, 192)
	cShadow = rgb(128, 128, 128)
	cBlack  = rgb(0, 0, 0)
	cGray   = rgb(181, 181, 181)
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	msimg32  = syscall.NewLazyDLL("msimg32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")

	pRegisterClassEx    = user32.NewProc("RegisterClassExW")
	pCreateWindowEx     = user32.NewProc("CreateWindowExW")
	pDestroyWindow      = user32.NewProc("DestroyWindow")
	pDefWindowProc      = user32.NewProc("DefWindowProcW")
	pGetMessage         = user32.NewProc("GetMessageW")
	pTranslateMessage   = user32.NewProc("TranslateMessage")
	pDispatchMessage    = user32.NewProc("DispatchMessageW")
	pIsDialogMessage    = user32.NewProc("IsDialogMessageW")
	pPostQuitMessage    = user32.NewProc("PostQuitMessage")
	pPostMessage        = user32.NewProc("PostMessageW")
	pSendMessage        = user32.NewProc("SendMessageW")
	pShowWindow         = user32.NewProc("ShowWindow")
	pUpdateWindow       = user32.NewProc("UpdateWindow")
	pGetDlgItem         = user32.NewProc("GetDlgItem")
	pGetWindowText      = user32.NewProc("GetWindowTextW")
	pGetWindowTextLen   = user32.NewProc("GetWindowTextLengthW")
	pSetWindowText      = user32.NewProc("SetWindowTextW")
	pSetFocus           = user32.NewProc("SetFocus")
	pLoadCursor         = user32.NewProc("LoadCursorW")
	pLoadImage          = user32.NewProc("LoadImageW")
	pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	pGetDC              = user32.NewProc("GetDC")
	pReleaseDC          = user32.NewProc("ReleaseDC")
	pBeginPaint         = user32.NewProc("BeginPaint")
	pEndPaint           = user32.NewProc("EndPaint")
	pInvalidateRect     = user32.NewProc("InvalidateRect")
	pFillRect           = user32.NewProc("FillRect")
	pDrawText           = user32.NewProc("DrawTextW")
	pDrawFocusRect      = user32.NewProc("DrawFocusRect")
	pDrawIconEx         = user32.NewProc("DrawIconEx")
	pScreenToClient     = user32.NewProc("ScreenToClient")
	pSystemParamsInfo   = user32.NewProc("SystemParametersInfoW")
	pSetTimer           = user32.NewProc("SetTimer")
	pKillTimer          = user32.NewProc("KillTimer")
	pOpenClipboard      = user32.NewProc("OpenClipboard")
	pEmptyClipboard     = user32.NewProc("EmptyClipboard")
	pSetClipboardData   = user32.NewProc("SetClipboardData")
	pCloseClipboard     = user32.NewProc("CloseClipboard")

	pGetModuleHandle = kernel32.NewProc("GetModuleHandleW")
	pGlobalAlloc     = kernel32.NewProc("GlobalAlloc")
	pGlobalLock      = kernel32.NewProc("GlobalLock")
	pGlobalUnlock    = kernel32.NewProc("GlobalUnlock")
	pGlobalFree      = kernel32.NewProc("GlobalFree")

	pCreateFont        = gdi32.NewProc("CreateFontW")
	pCreateSolidBrush  = gdi32.NewProc("CreateSolidBrush")
	pCreateCompatDC    = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatBmp   = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject      = gdi32.NewProc("SelectObject")
	pDeleteObject      = gdi32.NewProc("DeleteObject")
	pDeleteDC          = gdi32.NewProc("DeleteDC")
	pBitBlt            = gdi32.NewProc("BitBlt")
	pGetDeviceCaps     = gdi32.NewProc("GetDeviceCaps")
	pSetTextColor      = gdi32.NewProc("SetTextColor")
	pSetBkMode         = gdi32.NewProc("SetBkMode")
	pGradientFill      = msimg32.NewProc("GradientFill")
	pDwmSetWindowAttrb = dwmapi.NewProc("DwmSetWindowAttribute")
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

type rect struct{ left, top, right, bottom int32 }

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type drawItemStruct struct {
	ctlType, ctlID, itemID, itemAction, itemState uint32
	hwndItem, hDC                                 uintptr
	rcItem                                        rect
	itemData                                      uintptr
}

type triVertex struct {
	x, y       int32
	r, g, b, a uint16
}

var (
	hwndMain uintptr
	dpi      int32 = 96
	winW     int32
	winH     int32

	fontUI, fontBold, fontBig, fontMono uintptr
	iconBig, iconSmall                  uintptr
	brushes                             = map[uint32]uintptr{}

	template  = defaultTemplate
	active    = true
	status    = "Pronto."
	lastGreet string
)

func u16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

// sc converte pixel logici (96 DPI) in pixel reali.
func sc(n int32) int32 { return n * dpi / 96 }

// px è lo spessore delle linee 3D: 1 pixel, 2 sopra il 192 DPI.
func px() int32 {
	if dpi >= 192 {
		return 2
	}
	return 1
}

func brush(c uint32) uintptr {
	b, ok := brushes[c]
	if !ok {
		b, _, _ = pCreateSolidBrush.Call(uintptr(c))
		brushes[c] = b
	}
	return b
}

func fill(hdc uintptr, x, y, w, h int32, c uint32) {
	r := rect{x, y, x + w, y + h}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), brush(c))
}

// edge disegna un bordo di spessore px(): tl in alto/sinistra, br in basso/destra.
func edge(hdc uintptr, r rect, tl, br uint32) rect {
	p := px()
	w, h := r.right-r.left, r.bottom-r.top
	fill(hdc, r.left, r.top, w-p, p, tl)
	fill(hdc, r.left, r.top, p, h-p, tl)
	fill(hdc, r.left, r.bottom-p, w, p, br)
	fill(hdc, r.right-p, r.top, p, h, br)
	return rect{r.left + p, r.top + p, r.right - p, r.bottom - p}
}

func raised(hdc uintptr, r rect, pressed bool) rect {
	if pressed {
		r = edge(hdc, r, cBlack, cWhite)
		return edge(hdc, r, cShadow, cLight)
	}
	r = edge(hdc, r, cWhite, cBlack)
	return edge(hdc, r, cLight, cShadow)
}

func sunkenThin(hdc uintptr, r rect) rect { return edge(hdc, r, cShadow, cWhite) }

// etched disegna la cornice "incisa" dei group box: grigia con riflesso bianco.
func etched(hdc uintptr, x, y, w, h int32) {
	p := px()
	edge(hdc, rect{x + p, y + p, x + w, y + h}, cWhite, cWhite)
	edge(hdc, rect{x, y, x + w - p, y + h - p}, cShadow, cShadow)
}

func measure(hdc uintptr, s string, font uintptr) int32 {
	old, _, _ := pSelectObject.Call(hdc, font)
	var r rect
	pDrawText.Call(hdc, uintptr(unsafe.Pointer(u16(s))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x400|dtSingle|dtNoPrefix /*DT_CALCRECT*/)
	pSelectObject.Call(hdc, old)
	return r.right - r.left
}

func text(hdc uintptr, s string, r rect, font uintptr, color uint32, flags uint32) {
	old, _, _ := pSelectObject.Call(hdc, font)
	pSetBkMode.Call(hdc, 1)
	pSetTextColor.Call(hdc, uintptr(color))
	pDrawText.Call(hdc, uintptr(unsafe.Pointer(u16(s))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), uintptr(flags))
	pSelectObject.Call(hdc, old)
}

const (
	dtCenter   = 0x01
	dtRight    = 0x02
	dtVCenter  = 0x04
	dtSingle   = 0x20
	dtNoPrefix = 0x800
	dtEllipsis = 0x8000
)

func getText(h uintptr) string {
	n, _, _ := pGetWindowTextLen.Call(h)
	buf := make([]uint16, n+1)
	pGetWindowText.Call(h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func setText(h uintptr, s string) { pSetWindowText.Call(h, uintptr(unsafe.Pointer(u16(s)))) }

func item(id int) uintptr { h, _, _ := pGetDlgItem.Call(hwndMain, uintptr(id)); return h }

func invalidate(r *rect) { pInvalidateRect.Call(hwndMain, uintptr(unsafe.Pointer(r)), 0) }

// "mario rossi" -> "Mario Rossi"; lascia invariato ciò che l'operatore ha già maiuscolo.
func capitalize(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		r, size := utf8.DecodeRuneInString(w)
		words[i] = string(unicode.ToUpper(r)) + w[size:]
	}
	return strings.Join(words, " ")
}

func greeting() string {
	if time.Now().Hour() < 14 {
		return "Buongiorno"
	}
	return "Buonasera"
}

func buildMessage() (string, bool) {
	name := capitalize(getText(item(idName)))
	device := strings.TrimSpace(getText(item(idDevice)))
	ok := name != "" && device != ""
	if name == "" {
		name = "[NOME]"
	}
	if device == "" {
		device = "[DISPOSITIVO]"
	}
	r := strings.NewReplacer("[SALUTO]", greeting(), "[NOME]", name, "[DISPOSITIVO]", device)
	return r.Replace(template), ok
}

func crlf(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

func refreshPreview() {
	lastGreet = greeting()
	m, _ := buildMessage()
	setText(item(idPrev), crlf(m))
}

// ---- Layout (pixel logici a 96 DPI) ----

const (
	logW    = 470
	logH    = 540
	frame   = 4
	titleH  = 18
	headerH = 58
)

func titleRect() rect { return rect{sc(frame), sc(frame), winW - sc(frame), sc(frame + titleH)} }

func statusRect() rect {
	return rect{sc(frame + 2), winH - sc(frame+22), winW - sc(frame+2), winH - sc(frame+2)}
}

func setStatus(s string) {
	status = s
	pKillTimer.Call(hwndMain, timerStatus)
	if s != "Pronto." {
		pSetTimer.Call(hwndMain, timerStatus, 5000, 0)
	}
	r := statusRect()
	invalidate(&r)
}

func copyToClipboard(s string) bool {
	utf16, _ := syscall.UTF16FromString(s)
	size := uintptr(len(utf16) * 2)
	if r, _, _ := pOpenClipboard.Call(hwndMain); r == 0 {
		return false
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	h, _, _ := pGlobalAlloc.Call(0x0002 /*GMEM_MOVEABLE*/, size)
	if h == 0 {
		return false
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		pGlobalFree.Call(h)
		return false
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(utf16)), utf16)
	pGlobalUnlock.Call(h)
	if r, _, _ := pSetClipboardData.Call(13 /*CF_UNICODETEXT*/, h); r == 0 {
		pGlobalFree.Call(h)
		return false
	}
	return true
}

func onCopy() {
	m, ok := buildMessage()
	if !ok {
		setStatus("Compila nome e dispositivo prima di copiare.")
		if strings.TrimSpace(getText(item(idName))) == "" {
			pSetFocus.Call(item(idName))
		} else {
			pSetFocus.Call(item(idDevice))
		}
		return
	}
	if copyToClipboard(crlf(m)) {
		setStatus("Messaggio copiato! Incollalo su WhatsApp con Ctrl+V.")
	} else {
		setStatus("Impossibile accedere agli appunti, riprova.")
	}
}

func onNew() {
	setText(item(idName), "")
	setText(item(idDevice), "")
	setStatus("Pronto.")
	refreshPreview()
	pSetFocus.Call(item(idName))
}

func create(ex uint32, class, txt string, style uint32, x, y, w, h int32, id int, font uintptr) uintptr {
	hinst, _, _ := pGetModuleHandle.Call(0)
	hw, _, _ := pCreateWindowEx.Call(uintptr(ex),
		uintptr(unsafe.Pointer(u16(class))), uintptr(unsafe.Pointer(u16(txt))),
		uintptr(style|0x40000000 /*WS_CHILD*/ |0x10000000 /*WS_VISIBLE*/),
		uintptr(sc(x)), uintptr(sc(y)), uintptr(sc(w)), uintptr(sc(h)),
		hwndMain, uintptr(id), hinst, 0)
	pSendMessage.Call(hw, 0x0030 /*WM_SETFONT*/, font, 1)
	return hw
}

// Coordinate dei controlli, condivise tra creazione e disegno.
const (
	grp1Y   = frame + titleH + headerH + 12
	grp1H   = 90
	rowName = grp1Y + 22
	rowDev  = grp1Y + 54
	grp2Y   = grp1Y + grp1H + 10
	btnY    = logH - frame - 22 - 34
	grp2H   = btnY - 10 - grp2Y
	fieldX  = 112
)

func buildUI() {
	const (
		wsTabStop  = 0x00010000
		wsVScroll  = 0x00200000
		esAutoH    = 0x0080
		esMulti    = 0x0004
		esReadOnly = 0x0800
		esAutoV    = 0x0040
		wsExClient = 0x00000200
		bsOwner    = 0x000B
	)
	fieldW := int32(logW - 24 - fieldX)

	create(0, "BUTTON", "", bsOwner, logW-frame-2-16-2-16, frame+2, 16, 14, idMin, fontUI)
	create(0, "BUTTON", "", bsOwner, logW-frame-2-16, frame+2, 16, 14, idClose, fontUI)

	name := create(wsExClient, "EDIT", "", wsTabStop|esAutoH, fieldX, rowName-3, fieldW, 22, idName, fontUI)
	pSendMessage.Call(name, 0x00C5 /*EM_LIMITTEXT*/, 60, 0)

	cb := create(0, "COMBOBOX", "", wsTabStop|wsVScroll|0x0002 /*CBS_DROPDOWN*/ |0x0040 /*CBS_AUTOHSCROLL*/, fieldX, rowDev-3, fieldW, 240, idDevice, fontUI)
	for _, d := range devices {
		pSendMessage.Call(cb, 0x0143 /*CB_ADDSTRING*/, 0, uintptr(unsafe.Pointer(u16(d))))
	}

	create(wsExClient, "EDIT", "", esMulti|esReadOnly|esAutoV|wsVScroll, 22, grp2Y+20, logW-44, grp2H-32, idPrev, fontMono)

	create(0, "BUTTON", "Copia messaggio", wsTabStop|bsOwner, logW-12-104-8-128, btnY, 128, 26, idCopy, fontUI)
	create(0, "BUTTON", "Nuovo cliente", wsTabStop|bsOwner, logW-12-104, btnY, 104, 26, idNew, fontUI)

	refreshPreview()
	pSetFocus.Call(name)
}

// ---- Disegno ----

func paint(hdc uintptr) {
	S := sc
	fill(hdc, 0, 0, winW, winH, cFace)

	// Cornice della finestra.
	r := edge(hdc, rect{0, 0, winW, winH}, cLight, cBlack)
	edge(hdc, r, cWhite, cShadow)

	// Barra del titolo con gradiente.
	t := titleRect()
	from, to := [3]uint16{0x00, 0x00, 0x80}, [3]uint16{0x10, 0x84, 0xD0}
	if !active {
		from, to = [3]uint16{0x80, 0x80, 0x80}, [3]uint16{0xB5, 0xB5, 0xB5}
	}
	v := [2]triVertex{
		{t.left, t.top, from[0] << 8, from[1] << 8, from[2] << 8, 0},
		{t.right, t.bottom, to[0] << 8, to[1] << 8, to[2] << 8, 0},
	}
	idx := [2]uint32{0, 1}
	pGradientFill.Call(hdc, uintptr(unsafe.Pointer(&v[0])), 2, uintptr(unsafe.Pointer(&idx[0])), 1, 0 /*H*/)
	pDrawIconEx.Call(hdc, uintptr(t.left+S(2)), uintptr(t.top+S(1)), iconSmall, uintptr(S(16)), uintptr(S(16)), 0, 0, 3)
	titleColor := cWhite
	if !active {
		titleColor = cFace
	}
	text(hdc, appTitle, rect{t.left + S(21), t.top, t.right - S(40), t.bottom}, fontBold, titleColor, dtSingle|dtVCenter|dtNoPrefix|dtEllipsis)

	// Intestazione bianca stile installazione guidata.
	hy := t.bottom + S(1)
	fill(hdc, S(frame), hy, winW-S(2*frame), S(headerH), cWhite)
	text(hdc, appShort, rect{S(18), hy + S(3), winW, hy + S(31)}, fontBig, rgb(0, 0, 128), dtSingle|dtNoPrefix)
	text(hdc, appName, rect{S(20), hy + S(30), winW, hy + S(44)}, fontBold, cBlack, dtSingle|dtNoPrefix)
	text(hdc, "Messaggi di ritiro dispositivo per WhatsApp", rect{S(20), hy + S(43), winW, hy + S(57)}, fontUI, cShadow, dtSingle|dtNoPrefix)
	pDrawIconEx.Call(hdc, uintptr(winW-S(frame+16+32)), uintptr(hy+S(13)), iconBig, uintptr(S(32)), uintptr(S(32)), 0, 0, 3)
	fill(hdc, S(frame), hy+S(headerH), winW-S(2*frame), px(), cShadow)
	fill(hdc, S(frame), hy+S(headerH)+px(), winW-S(2*frame), px(), cWhite)

	// Group box.
	group := func(label string, y, h int32) {
		etched(hdc, S(12), S(y+6), winW-S(24), S(h-6))
		w := measure(hdc, label, fontUI) + S(6)
		fill(hdc, S(20), S(y), w, S(14), cFace)
		text(hdc, label, rect{S(23), S(y), S(20) + w, S(y + 14)}, fontUI, cBlack, dtSingle|dtVCenter|dtNoPrefix)
	}
	group("Dati cliente", grp1Y, grp1H)
	group("Anteprima messaggio", grp2Y, grp2H)
	text(hdc, "Nome cliente:", rect{S(24), S(rowName), S(fieldX - 4), S(rowName + 16)}, fontUI, cBlack, dtSingle|dtVCenter|dtNoPrefix)
	text(hdc, "Dispositivo:", rect{S(24), S(rowDev), S(fieldX - 4), S(rowDev + 16)}, fontUI, cBlack, dtSingle|dtVCenter|dtNoPrefix)

	// Barra di stato: messaggio + orologio.
	sr := statusRect()
	clockW := S(64)
	in := sunkenThin(hdc, rect{sr.left, sr.top, sr.right - clockW - S(2), sr.bottom})
	text(hdc, status, rect{in.left + S(4), in.top, in.right - S(4), in.bottom}, fontUI, cBlack, dtSingle|dtVCenter|dtNoPrefix|dtEllipsis)
	in = sunkenThin(hdc, rect{sr.right - clockW, sr.top, sr.right, sr.bottom})
	text(hdc, time.Now().Format("15:04"), in, fontUI, cBlack, dtSingle|dtVCenter|dtCenter|dtNoPrefix)
}

func onPaint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	// Doppio buffer: niente sfarfallio.
	mem, _, _ := pCreateCompatDC.Call(hdc)
	bmp, _, _ := pCreateCompatBmp.Call(hdc, uintptr(winW), uintptr(winH))
	old, _, _ := pSelectObject.Call(mem, bmp)
	paint(mem)
	pBitBlt.Call(hdc, 0, 0, uintptr(winW), uintptr(winH), mem, 0, 0, 0x00CC0020 /*SRCCOPY*/)
	pSelectObject.Call(mem, old)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(mem)
	pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

func drawButton(d *drawItemStruct) {
	const odsSelected, odsFocus = 0x1, 0x10
	hdc, r := d.hDC, d.rcItem
	pressed := d.itemState&odsSelected != 0
	fill(hdc, r.left, r.top, r.right-r.left, r.bottom-r.top, cFace)
	off := int32(0)
	if pressed {
		off = px()
	}

	switch d.ctlID {
	case idMin, idClose:
		raised(hdc, r, pressed)
		u := sc(1)
		cx := (r.left+r.right)/2 - 4*u + off
		cy := (r.top+r.bottom)/2 - 4*u + off
		if d.ctlID == idClose {
			for i := int32(0); i < 7; i++ {
				fill(hdc, cx+i*u, cy+i*u, 2*u, u, cBlack)
				fill(hdc, cx+(6-i)*u, cy+i*u, 2*u, u, cBlack)
			}
		} else {
			fill(hdc, cx+u, cy+6*u, 6*u, 2*u, cBlack)
		}
		return
	}

	if d.ctlID == idCopy { // pulsante predefinito: bordo nero esterno
		r = edge(hdc, r, cBlack, cBlack)
	}
	inner := raised(hdc, r, pressed)
	label := getText(d.hwndItem)
	tr := rect{inner.left + off, inner.top + off, inner.right + off, inner.bottom + off}
	text(hdc, label, tr, fontUI, cBlack, dtSingle|dtVCenter|dtCenter|dtNoPrefix)
	if d.itemState&odsFocus != 0 {
		f := rect{inner.left + sc(3), inner.top + sc(3), inner.right - sc(3), inner.bottom - sc(3)}
		pDrawFocusRect.Call(hdc, uintptr(unsafe.Pointer(&f)))
	}
}

func wndProc(hwnd uintptr, m uint32, wp, lp uintptr) uintptr {
	switch m {
	case 0x000F: // WM_PAINT
		onPaint(hwnd)
		return 0
	case 0x0014: // WM_ERASEBKGND
		return 1
	case 0x0084: // WM_NCHITTEST: la barra del titolo trascina la finestra
		pt := [2]int32{int32(int16(lp & 0xFFFF)), int32(int16(lp >> 16 & 0xFFFF))}
		pScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
		t := titleRect()
		if pt[0] >= t.left && pt[0] < t.right && pt[1] >= t.top && pt[1] < t.bottom {
			return 2 // HTCAPTION
		}
		return 1 // HTCLIENT
	case 0x00A3: // WM_NCLBUTTONDBLCLK: niente massimizza
		return 0
	case 0x0006: // WM_ACTIVATE
		active = wp&0xFFFF != 0
		t := titleRect()
		invalidate(&t)
	case 0x0400: // DM_GETDEFID: Invio = Copia
		return 0x534B<<16 | idCopy
	case 0x002B: // WM_DRAWITEM
		drawButton((*drawItemStruct)(unsafe.Pointer(lp)))
		return 1
	case 0x0111: // WM_COMMAND
		id, code := int(wp&0xFFFF), uint32(wp>>16)
		switch {
		case id == idCopy && code == 0:
			onCopy()
		case id == idNew && code == 0:
			onNew()
		case id == idMin && code == 0:
			pShowWindow.Call(hwnd, 6 /*SW_MINIMIZE*/)
		case id == idClose && code == 0:
			pDestroyWindow.Call(hwnd)
		case id == idName && code == 0x0300: // EN_CHANGE
			refreshPreview()
		case id == idDevice && (code == 5 || code == 1): // CBN_EDITCHANGE / CBN_SELCHANGE
			// Su SELCHANGE il testo non è ancora aggiornato: rimando il refresh.
			pPostMessage.Call(hwnd, wmApp, 0, 0)
		}
		return 0
	case wmApp:
		refreshPreview()
		return 0
	case 0x0113: // WM_TIMER
		if wp == timerStatus {
			setStatus("Pronto.")
		} else {
			if greeting() != lastGreet {
				refreshPreview()
			}
			r := statusRect()
			invalidate(&r)
		}
		return 0
	case 0x0133, 0x0134: // WM_CTLCOLOREDIT, WM_CTLCOLORLISTBOX
		pSetTextColor.Call(wp, uintptr(cBlack))
		return brush(cWhite)
	case 0x0138: // WM_CTLCOLORSTATIC (anteprima in sola lettura)
		pSetTextColor.Call(wp, uintptr(cBlack))
		return brush(cWhite)
	case 0x0002: // WM_DESTROY
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, uintptr(m), wp, lp)
	return r
}

func loadTemplate() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "messaggio.txt"))
	if err != nil {
		return
	}
	s := strings.TrimPrefix(string(b), "\uFEFF")
	if strings.TrimSpace(s) != "" {
		template = strings.TrimRight(s, "\r\n ")
	}
}

func mkFont(name string, size int32, weight int) uintptr {
	f, _, _ := pCreateFont.Call(uintptr(-sc(size)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0,
		3 /*NONANTIALIASED_QUALITY: pixel puri come nel '98*/, 0, uintptr(unsafe.Pointer(u16(name))))
	return f
}

func main() {
	runtime.LockOSThread()
	loadTemplate()

	pSetProcessDPIAware.Call()
	if hdc, _, _ := pGetDC.Call(0); hdc != 0 {
		d, _, _ := pGetDeviceCaps.Call(hdc, 90 /*LOGPIXELSY*/)
		dpi = int32(d)
		pReleaseDC.Call(0, hdc)
	}
	winW, winH = sc(logW), sc(logH)
	fontUI = mkFont("MS Sans Serif", 11, 400)
	fontBold = mkFont("MS Sans Serif", 11, 700)
	fontBig = mkFont("Arial", 26, 900)
	fontMono = mkFont("Courier New", 13, 400)

	hinst, _, _ := pGetModuleHandle.Call(0)
	cursor, _, _ := pLoadCursor.Call(0, 32512 /*IDC_ARROW*/)
	// rsrc assegna l'ID 1 al manifest e il 2 al gruppo di icone.
	const iconID = 2
	iconBig, _, _ = pLoadImage.Call(hinst, iconID, 1 /*IMAGE_ICON*/, uintptr(sc(32)), uintptr(sc(32)), 0)
	iconSmall, _, _ = pLoadImage.Call(hinst, iconID, 1, uintptr(sc(16)), uintptr(sc(16)), 0)
	className := u16("AutomatedSupportSystem")
	wc := wndClassEx{
		style:         0x0008, // CS_DBLCLKS
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     hinst,
		hIcon:         iconBig,
		hIconSm:       iconSmall,
		hCursor:       cursor,
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	// Centrata nell'area di lavoro.
	var wa rect
	pSystemParamsInfo.Call(0x0030 /*SPI_GETWORKAREA*/, 0, uintptr(unsafe.Pointer(&wa)), 0)
	x := wa.left + (wa.right-wa.left-winW)/2
	y := wa.top + (wa.bottom-wa.top-winH)/2

	const style = 0x80000000 | 0x00080000 | 0x00020000 | 0x02000000 // POPUP | SYSMENU | MINIMIZEBOX | CLIPCHILDREN
	hwndMain, _, _ = pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(u16(appTitle))), style,
		uintptr(x), uintptr(y), uintptr(winW), uintptr(winH), 0, 0, hinst, 0)

	// Windows 11: angoli squadrati, come si deve.
	if dwmapi.Load() == nil && pDwmSetWindowAttrb.Find() == nil {
		pref := int32(1) // DWMWCP_DONOTROUND
		pDwmSetWindowAttrb.Call(hwndMain, 33 /*DWMWA_WINDOW_CORNER_PREFERENCE*/, uintptr(unsafe.Pointer(&pref)), 4)
	}

	buildUI()
	pSetTimer.Call(hwndMain, timerClock, 10000, 0)
	pShowWindow.Call(hwndMain, 1)
	pUpdateWindow.Call(hwndMain)

	var m msg
	for {
		ret, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		if d, _, _ := pIsDialogMessage.Call(hwndMain, uintptr(unsafe.Pointer(&m))); d != 0 {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}
