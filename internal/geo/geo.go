package geo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"xray-tui/internal/config"
)

type Status struct {
	HasSite    bool
	HasIP      bool
	TotalBytes int64
	UpdatedAt  time.Time
}

func (s Status) Ready() bool {
	return s.HasSite && s.HasIP
}

func Check(dir string) Status {
	var st Status
	if info, err := os.Stat(filepath.Join(dir, "geosite.dat")); err == nil && info.Size() > 0 {
		st.HasSite = true
		st.TotalBytes += info.Size()
		st.UpdatedAt = info.ModTime()
	}
	if info, err := os.Stat(filepath.Join(dir, "geoip.dat")); err == nil && info.Size() > 0 {
		st.HasIP = true
		st.TotalBytes += info.Size()
		if info.ModTime().After(st.UpdatedAt) {
			st.UpdatedAt = info.ModTime()
		}
	}
	return st
}

func Download(ctx context.Context, p config.GeoProvider, dir string) (Status, error) {
	if p.GeositeURL == "" || p.GeoipURL == "" {
		return Status{}, fmt.Errorf("у провайдера %q не заданы URL для geosite.dat или geoip.dat", p.Name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Status{}, fmt.Errorf("не удалось создать %s: %w", dir, err)
	}

	client := &http.Client{Timeout: 90 * time.Second}

	if err := downloadFile(ctx, client, p.GeositeURL, filepath.Join(dir, "geosite.dat")); err != nil {
		return Status{}, fmt.Errorf("geosite.dat: %w", err)
	}
	if err := downloadFile(ctx, client, p.GeoipURL, filepath.Join(dir, "geoip.dat")); err != nil {
		return Status{}, fmt.Errorf("geoip.dat: %w", err)
	}

	return Check(dir), nil
}

func downloadFile(ctx context.Context, client *http.Client, fileURL, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Fluffify/1.0.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpPath := destPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	n, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	if n == 0 {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("получен пустой файл")
	}

	return os.Rename(tmpPath, destPath)
}
