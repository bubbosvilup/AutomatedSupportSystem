# FlashinLabs – Messaggi clienti

App Windows portable (un solo `.exe`, nessuna installazione) che genera il messaggio
"dispositivo pronto per il ritiro" da copiare e incollare su WhatsApp Business.

Scritta in Go chiamando direttamente le API Win32: nessuna dipendenza esterna.

## Uso

1. Scrivi il nome del cliente e scegli/scrivi il dispositivo.
2. Premi **Copia messaggio** (o Invio) e incolla su WhatsApp con Ctrl+V.
3. **Nuovo cliente** svuota i campi.

- Il saluto è automatico: "Buongiorno" fino alle 14:00, poi "Buonasera".
- Il nome viene capitalizzato (`mario rossi` → `Mario Rossi`).
- Per cambiare il testo senza ricompilare, metti un file `messaggio.txt` accanto all'exe
  (vedi `messaggio.esempio.txt`). Segnaposto: `[SALUTO]`, `[NOME]`, `[DISPOSITIVO]`.

## Build

Richiede Go 1.24+. Da qualunque sistema operativo:

```sh
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui" -o FlashinLabs-Messaggi.exe .
```

Su Windows (PowerShell):

```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"
go build -trimpath -ldflags "-s -w -H windowsgui" -o FlashinLabs-Messaggi.exe .
```

`rsrc_windows_amd64.syso` incorpora icona e manifest (stili visivi moderni, DPI-aware).
Per rigenerarlo dopo aver modificato `icon.ico` o `app.manifest`:

```sh
go install github.com/akavel/rsrc@latest
rsrc -manifest app.manifest -ico icon.ico -arch amd64 -o rsrc_windows_amd64.syso
```
