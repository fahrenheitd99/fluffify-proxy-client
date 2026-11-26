// Package engine запускает Xray-core внутри нашего процесса.
//
// Здесь нет ни одного внешнего процесса: конфиг разбирается в память
// (infra/conf/serial), инстанс создаётся через core.New и поднимается
// Instance.Start(). Остановка — обычный вызов Instance.Close(), никакого
// ожидания ответа от дочернего процесса.
//
// TUN тоже создаёт само ядро (inbound с protocol "tun"): на Windows — через
// wintun.dll, на Linux — через /dev/net/tun и netlink. Опции fd у tun-инбаунда
// в Xray нет, поэтому дескриптор снаружи не передаётся; зато права нужны уже
// нашему процессу: администратор в Windows, CAP_NET_ADMIN в Linux.
package engine

import (
	"bytes"
	"fmt"
	"os"
	"sync"

	applog "github.com/xtls/xray-core/app/log"
	clog "github.com/xtls/xray-core/common/log"
	"github.com/xtls/xray-core/common/platform"
	xcore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"

	"xray-tui/internal/osutil"
)

// Options — настройки движка.
type Options struct {
	// OnLog получает уже отформатированные строки лога ядра. Уровень
	// фильтруется самим ядром по loglevel из конфига.
	OnLog func(line string)
}

// Engine управляет жизненным циклом Xray внутри текущего процесса.
type Engine struct {
	mu       sync.Mutex
	instance *xcore.Instance
	onLog    func(string)
}

// New создаёт движок; ядро стартует только по вызову Start.
func New(opts Options) *Engine {
	// nil не затирает приёмник: временное ядро (например, для замера ping)
	// не должно отбирать логи у основного инстанса.
	if opts.OnLog != nil {
		sink.set(opts.OnLog)
	}
	return &Engine{onLog: opts.OnLog}
}

// warn пишет сообщение в приёмник логов, если он подключён.
func (e *Engine) warn(line string) {
	if e.onLog != nil {
		e.onLog(line)
	}
}

// Start поднимает Xray из JSON-конфига в этом процессе. Если ядро уже
// запущено, предыд��щий инстанс сначала останавливается.
func (e *Engine) Start(configJSON []byte, assetDir string) error {
	if len(bytes.TrimSpace(configJSON)) == 0 {
		return fmt.Errorf("пустой конфиг Xray")
	}

	// Каталог геобаз задаём переменной окружения: ядро живёт в нашем процессе,
	// а не в отдельном xray.exe, поэтому env ставим себе.
	if assetDir != "" {
		_ = os.Setenv(platform.AssetLocation, assetDir)
	}

	// Перед созданием ядра готовим окружение: в Windows драйвер TUN
	// (wintun.dll) извлекается/предзагружается, если его нет рядом с exe.
	osutil.EnsureWintunDLL()

	// Снимаем работающий инстанс до создания нового: сокеты и TUN-адаптер
	// должны освободиться, иначе займём те же порты дважды.
	//
	// Важно: Xray возвращает ошибку Close, если хотя бы одна фича не
	// закрылась. Прерывать из-за этого запуск нового инстанса нельзя —
	// именно поэтому повторное включение TUN раньше падало. Ядро при этом
	// остановлено, предупреждение уходит в лог.
	_ = e.Stop()

	cfg, err := serial.LoadJSONConfig(bytes.NewReader(configJSON))
	if err != nil {
		return fmt.Errorf("разбор конфига Xray: %w", err)
	}

	instance, err := xcore.New(cfg)
	if err != nil {
		return fmt.Errorf("создание ядра: %w", err)
	}
	if err := instance.Start(); err != nil {
		_ = instance.Close()
		return fmt.Errorf("запуск ядра: %w", err)
	}

	e.mu.Lock()
	e.instance = instance
	e.mu.Unlock()
	return nil
}

// Stop закрывает инстанс Xray, освобождая порты и TUN-адаптер. Ядро
// останавливается всегда; ошибки закрытия отдельных фич не прерывают
// остановку и уходят в лог — следующий Start должен работать.
func (e *Engine) Stop() error {
	e.mu.Lock()
	instance := e.instance
	e.instance = nil
	e.mu.Unlock()

	if instance == nil {
		return nil
	}
	if err := instance.Close(); err != nil {
		e.warn("предыдущее ядро остановлено с предупреждением: " + err.Error())
	}
	return nil
}

// Running сообщает, поднято ли ядро.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.instance != nil
}

// logSink — приёмник логов ядра. Xray создаёт свой обработчик на каждый
// инстанс, забирая его из реестра app/log, поэтому колбэк хранится в одном
// месте и подменяется при создании движка.
var sink = &logSink{}

type logSink struct {
	mu   sync.RWMutex
	emit func(string)
}

func (s *logSink) Handle(msg clog.Message) {
	s.mu.RLock()
	emit := s.emit
	s.mu.RUnlock()
	if emit != nil {
		emit(msg.String())
	}
}

func (s *logSink) set(emit func(string)) {
	s.mu.Lock()
	s.emit = emit
	s.mu.Unlock()
}

func init() {
	// Забираем консольный лог ядра себе: app/log спрашивает обработчик у
	// реестра, а уровень (loglevel из конфига) фильтрует сам.
	_ = applog.RegisterHandlerCreator(applog.LogType_Console,
		func(applog.LogType, applog.HandlerCreatorOptions) (clog.Handler, error) {
			return sink, nil
		})
}
