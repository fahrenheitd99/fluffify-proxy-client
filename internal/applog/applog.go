// Package applog — структурный журнал действий приложения.
//
// Пишется всегда (не только при открытом окне логов) в кольцевой файл
// с ротацией: app.log — активный, app.log.1 — предыдущий. Один файл
// используют все компоненты (демон, TUI, трей) — записи идут через
// O_APPEND, поэтому атомарны; сообщения на английском.
package applog

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// maxFileBytes — размер активного файла, при превышении которого журнал
// ротируется. Достаточно мало, чтобы не расти бесконечно, и достаточно
// много, чтобы вместить несколько часов отладки.
const maxFileBytes = 2 << 20

var (
	mu   sync.Mutex
	f    *os.File
	size int64
	lg   *slog.Logger = slog.New(slog.NewTextHandler(discard{}, &slog.HandlerOptions{Level: slog.LevelDebug}))
)

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// Dir возвращает каталог журналов (XDG_STATE_HOME или %LOCALAPPDATA%).
func Dir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "xray-tui")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".xray-tui"
	}
	// на Windows XDG_STATE_HOME обычно не задан, поэтому сразу домашний каталог
	if homeState := filepath.Join(home, ".local", "state"); runtime.GOOS != "windows" {
		return filepath.Join(homeState, "xray-tui")
	}
	return filepath.Join(home, "AppData", "Local", "xray-tui", "logs")
}

// File — путь активного журнала (для tail -f во внешнем терминале).
func File() string { return filepath.Join(Dir(), "app.log") }

// Init открывает журнал и назначает логгер компонента. component —
// короткое имя процесса ("daemon", "tui", "tray"), попадает в каждую запись.
func Init(component string) {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return
	}

	mu.Lock()
	defer mu.Unlock()
	if f != nil {
		_ = f.Close()
	}
	fp, err := os.OpenFile(File(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	st, err := fp.Stat()
	if err != nil {
		_ = fp.Close()
		return
	}
	f = fp
	size = st.Size()
	var h slog.Handler = slog.NewTextHandler(&rotWriter{}, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	if component != "" {
		h = h.WithAttrs([]slog.Attr{slog.String("component", component)})
	}
	lg = slog.New(h)
}

// L возвращает текущий логгер. До Init записываются в никуда.
func L() *slog.Logger { return lg }

// rotWriter оборачивает запись в файл ротацией по размеру.
type rotWriter struct{}

func (w *rotWriter) Write(p []byte) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	if f == nil {
		return len(p), nil
	}
	if size > 0 && size+int64(len(p)) > maxFileBytes {
		if err := rotate(); err != nil {
			// Ротация не удалась (конкурентная запись второго процесса):
			// продолжаем писать в тот же файл, попробуем снова позже.
			size = 0
		}
	}
	n, err := f.Write(p)
	size += int64(n)
	return n, err
}

// rotate вызывает ротацию; mu уже удержан.
func rotate() error {
	if f != nil {
		_ = f.Close()
		f = nil
	}
	base := Dir()
	_ = os.Remove(filepath.Join(base, "app.log.1"))
	if err := os.Rename(File(), filepath.Join(base, "app.log.1")); err != nil && !os.IsNotExist(err) {
		fp, openErr := os.OpenFile(File(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if openErr != nil {
			return openErr
		}
		f = fp
		return nil
	}
	fp, err := os.OpenFile(File(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	f = fp
	size = 0
	return nil
}

// Event — короткая обёртка для однострочных событий с длительностью.
func Event(msg string, args ...any) {
	L().Info(msg, args...)
}

// Fail — ошибка с context; msg на английском.
func Fail(msg string, err error, args ...any) {
	L().Error(msg, append(args, "err", err)...)
}

// Stamp — человекочитаемая метка для файлов отчётов.
func Stamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
