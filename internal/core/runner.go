package core

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const defaultLogLimit = 200

type Runner struct {
	binPath string
	onLog   func(line string)

	mu       sync.Mutex
	cmd      *exec.Cmd
	running  bool
	done     chan struct{}
	logs     []string
	logLimit int
}

func NewRunner(binPath string, onLog func(line string)) *Runner {
	if binPath == "" {
		binPath = "xray"
	}
	return &Runner{
		binPath:  binPath,
		onLog:    onLog,
		logLimit: defaultLogLimit,
	}
}

func (r *Runner) Start(configJSON []byte, assetDir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return errors.New("xray уже запущен")
	}

	cmd := exec.Command(r.binPath, "run", "-c", "stdin:")
	cmd.Stdin = bytes.NewReader(configJSON)
	if assetDir != "" {
		cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+assetDir)
	}

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		_ = pr.Close()
		return fmt.Errorf("не удалось запустить %s: %w", r.binPath, err)
	}

	done := make(chan struct{})
	r.cmd = cmd
	r.running = true
	r.done = done

	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			r.appendLog(scanner.Text())
		}
	}()

	go func() {
		err := cmd.Wait()
		_ = pw.Close()
		<-scanDone
		_ = pr.Close()

		if err != nil {
			r.appendLog(fmt.Sprintf("[runner] xray завершился с ошибкой: %v", err))
		} else {
			r.appendLog("[runner] xray остановлен")
		}

		r.mu.Lock()
		r.running = false
		r.cmd = nil
		r.mu.Unlock()

		close(done)
	}()

	return nil

}

func (r *Runner) Stop() error {
	r.mu.Lock()
	if !r.running || r.cmd == nil || r.cmd.Process == nil {
		r.mu.Unlock()
		return nil
	}
	proc := r.cmd.Process
	done := r.done
	r.mu.Unlock() // обязательно отпускаем мьютекс до ожидания done

	_ = proc.Signal(syscall.SIGTERM)

	select {
	case <-done:
		return nil
	case <-time.After(3 * time.Second):
		_ = proc.Kill()
		<-done
		return nil
	}
}

func (r *Runner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

func (r *Runner) Logs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.logs))
	copy(out, r.logs)
	return out
}

func (r *Runner) appendLog(line string) {
	r.mu.Lock()
	r.logs = append(r.logs, line)
	if len(r.logs) > r.logLimit {
		r.logs = r.logs[len(r.logs)-r.logLimit:]
	}
	cb := r.onLog
	r.mu.Unlock()

	if cb != nil {
		cb(line)
	}
}
