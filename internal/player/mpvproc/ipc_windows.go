//go:build windows

package mpvproc

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
)

// pipeSeq 保证同进程内命名管道名唯一。
var pipeSeq atomic.Uint64

// newIPCPath 返回一个唯一的命名管道基础名（不带 \\.\pipe\ 前缀）。mpv 的
// --input-ipc-server 在 Windows 上创建命名管道，并自行加 \\.\pipe\ 前缀。
func newIPCPath() (string, error) {
	return fmt.Sprintf("unbox-mpv-%d", pipeSeq.Add(1)), nil
}

// dialIPC 单次尝试连接 mpv 的命名管道，客户端用 os.OpenFile 打开
// \\.\pipe\<name>（等价 CreateFile 的 client 端）。重试与超时由 waitForIPC
// 统一处理——它还要在重试期间监视 mpv 是否已经退出。
func dialIPC(path string) (io.ReadWriteCloser, error) {
	return os.OpenFile(`\\.\pipe\`+path, os.O_RDWR, 0)
}

// cleanupIPC 命名管道随 mpv 退出自动销毁，无需显式删除。
func cleanupIPC(path string) {}

// setupProcAttr 隐藏 mpv 子进程的控制台窗口（mpv.exe 是控制台程序，否则从
// GUI 应用启动会弹出一个黑色终端）。CREATE_NO_WINDOW 禁止创建控制台。
func setupProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
