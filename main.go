//go:build windows

// FlashinLabs Messaggi — genera il messaggio "dispositivo pronto per il ritiro"
// da copiare e incollare su WhatsApp Business.
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
	idStatus = 106

	wmApp = 0x8000
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")

	pRegisterClassEx    = user32.NewProc("RegisterClassExW")
	pCreateWindowEx     = user32.NewProc("CreateWindowExW")
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
	pLoadIcon           = user32.NewProc("LoadIconW")
	pGetSysColorBrush   = user32.NewProc("GetSysColorBrush")
	pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	pGetDC              = user32.NewProc("GetDC")
	pReleaseDC          = user32.NewProc("ReleaseDC")
	pSetTimer           = user32.NewProc("SetTimer")
	pKillTimer          = user32.NewProc("KillTimer")
	pAdjustWindowRect   = user32.NewProc("AdjustWindowRect")
	pOpenClipboard      = user32.NewProc("OpenClipboard")
	pEmptyClipboard     = user32.NewProc("EmptyClipboard")
	pSetClipboardData   = user32.NewProc("SetClipboardData")
	pCloseClipboard     = user32.NewProc("CloseClipboard")

	pGetModuleHandle = kernel32.NewProc("GetModuleHandleW")
	pGlobalAlloc     = kernel32.NewProc("GlobalAlloc")
	pGlobalLock      = kernel32.NewProc("GlobalLock")
	pGlobalUnlock    = kernel32.NewProc("GlobalUnlock")
	pGlobalFree      = kernel32.NewProc("GlobalFree")

	pCreateFont    = gdi32.NewProc("CreateFontW")
	pGetDeviceCaps = gdi32.NewProc("GetDeviceCaps")
	pSetTextColor  = gdi32.NewProc("SetTextColor")
	pSetBkMode     = gdi32.NewProc("SetBkMode")
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

var (
	hwndMain   uintptr
	dpi        int32 = 96
	font       uintptr
	fontBold   uintptr
	template   = defaultTemplate
	statusIsOK bool
)

func u16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

func sc(n int32) int32 { return n * dpi / 96 }

func getText(h uintptr) string {
	n, _, _ := pGetWindowTextLen.Call(h)
	buf := make([]uint16, n+1)
	pGetWindowText.Call(h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func setText(h uintptr, s string) { pSetWindowText.Call(h, uintptr(unsafe.Pointer(u16(s)))) }

func item(id int) uintptr { h, _, _ := pGetDlgItem.Call(hwndMain, uintptr(id)); return h }

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
	m, _ := buildMessage()
	setText(item(idPrev), crlf(m))
}

func setStatus(s string, ok bool) {
	statusIsOK = ok
	setText(item(idStatus), s)
	pKillTimer.Call(hwndMain, 1)
	if s != "" {
		pSetTimer.Call(hwndMain, 1, 4000, 0)
	}
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
		setStatus("Compila nome e dispositivo prima di copiare.", false)
		if strings.TrimSpace(getText(item(idName))) == "" {
			pSetFocus.Call(item(idName))
		} else {
			pSetFocus.Call(item(idDevice))
		}
		return
	}
	if copyToClipboard(crlf(m)) {
		setStatus("Messaggio copiato! Incollalo su WhatsApp con Ctrl+V.", true)
	} else {
		setStatus("Impossibile accedere agli appunti, riprova.", false)
	}
}

func onNew() {
	setText(item(idName), "")
	setText(item(idDevice), "")
	setStatus("", true)
	refreshPreview()
	pSetFocus.Call(item(idName))
}

func create(ex uint32, class, text string, style uint32, x, y, w, h int32, id int) uintptr {
	hinst, _, _ := pGetModuleHandle.Call(0)
	hw, _, _ := pCreateWindowEx.Call(uintptr(ex),
		uintptr(unsafe.Pointer(u16(class))), uintptr(unsafe.Pointer(u16(text))),
		uintptr(style|0x40000000 /*WS_CHILD*/ |0x10000000 /*WS_VISIBLE*/),
		uintptr(sc(x)), uintptr(sc(y)), uintptr(sc(w)), uintptr(sc(h)),
		hwndMain, uintptr(id), hinst, 0)
	pSendMessage.Call(hw, 0x0030 /*WM_SETFONT*/, font, 1)
	return hw
}

const (
	winW = 520
	winH = 534
	pad  = 16
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
	)
	full := int32(winW - 2*pad)

	t := create(0, "STATIC", "Messaggio di ritiro dispositivo", 0, pad, 12, full, 26, 0)
	pSendMessage.Call(t, 0x0030, fontBold, 1)

	create(0, "STATIC", "Nome cliente", 0, pad, 48, 230, 20, 0)
	create(0, "STATIC", "Dispositivo", 0, pad+246, 48, 230, 20, 0)

	name := create(wsExClient, "EDIT", "", wsTabStop|esAutoH, pad, 70, 230, 26, idName)
	pSendMessage.Call(name, 0x00C5 /*EM_LIMITTEXT*/, 60, 0)

	cb := create(0, "COMBOBOX", "", wsTabStop|wsVScroll|0x0002 /*CBS_DROPDOWN*/ |0x0040 /*CBS_AUTOHSCROLL*/, pad+246, 70, full-246, 300, idDevice)
	for _, d := range devices {
		pSendMessage.Call(cb, 0x0143 /*CB_ADDSTRING*/, 0, uintptr(unsafe.Pointer(u16(d))))
	}

	create(0, "STATIC", "Anteprima", 0, pad, 110, full, 20, 0)
	create(wsExClient, "EDIT", "", esMulti|esReadOnly|esAutoV|wsVScroll, pad, 132, full, 300, idPrev)

	create(0, "BUTTON", "Copia messaggio", wsTabStop|0x0001 /*BS_DEFPUSHBUTTON*/, pad, 446, 200, 36, idCopy)
	create(0, "BUTTON", "Nuovo cliente", wsTabStop, pad+212, 446, 140, 36, idNew)

	create(0, "STATIC", "", 0, pad, 494, full, 40, idStatus)

	refreshPreview()
	pSetFocus.Call(name)
}

func wndProc(hwnd uintptr, m uint32, wp, lp uintptr) uintptr {
	switch m {
	case 0x0111: // WM_COMMAND
		id, code := int(wp&0xFFFF), uint32(wp>>16)
		switch {
		case id == idCopy && code == 0:
			onCopy()
		case id == idNew && code == 0:
			onNew()
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
		pKillTimer.Call(hwnd, 1)
		setText(item(idStatus), "")
		return 0
	case 0x0138: // WM_CTLCOLORSTATIC
		hdc := wp
		pSetBkMode.Call(hdc, 1 /*TRANSPARENT*/)
		if lp == item(idStatus) {
			if statusIsOK {
				pSetTextColor.Call(hdc, 0x00308000) // verde (BGR)
			} else {
				pSetTextColor.Call(hdc, 0x002020C0) // rosso (BGR)
			}
		}
		if lp == item(idPrev) {
			b, _, _ := pGetSysColorBrush.Call(5 /*COLOR_WINDOW*/)
			return b
		}
		b, _, _ := pGetSysColorBrush.Call(15 /*COLOR_BTNFACE*/)
		return b
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

func main() {
	runtime.LockOSThread()
	loadTemplate()

	pSetProcessDPIAware.Call()
	if hdc, _, _ := pGetDC.Call(0); hdc != 0 {
		d, _, _ := pGetDeviceCaps.Call(hdc, 90 /*LOGPIXELSY*/)
		dpi = int32(d)
		pReleaseDC.Call(0, hdc)
	}
	font, _, _ = pCreateFont.Call(uintptr(-sc(15)), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(u16("Segoe UI"))))
	fontBold, _, _ = pCreateFont.Call(uintptr(-sc(19)), 0, 0, 0, 600, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(u16("Segoe UI"))))

	hinst, _, _ := pGetModuleHandle.Call(0)
	cursor, _, _ := pLoadCursor.Call(0, 32512 /*IDC_ARROW*/)
	icon, _, _ := pLoadIcon.Call(hinst, 1) // icona incorporata (resource id 1)
	bg, _, _ := pGetSysColorBrush.Call(15)
	className := u16("FlashinLabsMessaggi")
	wc := wndClassEx{
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     hinst,
		hIcon:         icon,
		hIconSm:       icon,
		hCursor:       cursor,
		hbrBackground: bg,
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	const style = 0x00C00000 | 0x00080000 | 0x00020000 // CAPTION | SYSMENU | MINIMIZEBOX
	r := rect{0, 0, sc(winW), sc(winH)}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&r)), style, 0)
	hwndMain, _, _ = pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(u16("FlashinLabs – Messaggi clienti"))), style,
		0x80000000, 0x80000000, uintptr(r.right-r.left), uintptr(r.bottom-r.top), // CW_USEDEFAULT
		0, 0, hinst, 0)

	buildUI()
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
