//go:build !windows

package mpvproc

import (
	"io"
	"net"
	"os"
	"os/exec"
)

// newIPCPath 生成一个唯一的 Unix socket 路径（供 --input-ipc-server 使用）。
func newIPCPath() (string, error) {
	sock, err := os.CreateTemp("", "unbox-mpv-*.sock")
	if err != nil {
		return "", err
	}
	path := sock.Name()
	_ = sock.Close()
	_ = os.Remove(path)
	return path, nil
}

// dialIPC 单次尝试连接 mpv IPC socket。重试与超时由 waitForIPC 统一处理——
// 它还要在重试期间监视 mpv 是否已经退出，逻辑不宜留在平台文件里。
func dialIPC(path string) (io.ReadWriteCloser, error) {
	return net.Dial("unix", path)
}

// cleanupIPC 删除 Unix socket 文件。
func cleanupIPC(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

// setupProcAttr 非 Windows 平台无需隐藏控制台，空实现。
func setupProcAttr(cmd *exec.Cmd) {}
