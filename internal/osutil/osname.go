package osutil

import "runtime"

// OSName — человекочитаемое имя ОС для заголовков подписок и логов.
func OSName() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}
