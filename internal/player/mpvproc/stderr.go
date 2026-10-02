package mpvproc

import (
	"fmt"
	"strings"
	"sync"
)

// mpvStderrLimit 是保留的 mpv stderr 末尾字节数。取尾部而非头部：崩溃原因
// 通常紧跟在最后几行，而开头的版本横幅没有诊断价值。
const mpvStderrLimit = 4 << 10

// stderrTail 收集子进程 stderr 的末尾若干字节。
//
// 并发安全：写入方是 exec 的复制 goroutine，读取方是 Load 的失败路径。
// 读取只发生在 cmd.Wait 返回之后，此时复制 goroutine 已结束。
type stderrTail struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (s *stderrTail) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, p...)
	if len(s.buf) > s.max {
		s.buf = s.buf[len(s.buf)-s.max:]
	}
	// 必须汇报写入了全部字节：报少了 io.Copy 会当成写错误。
	return len(p), nil
}

func (s *stderrTail) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimSpace(string(s.buf))
}

// mpvLoadError 在 Load 的失败原因后附上 mpv 自己打印的输出。
//
// mpv 起不来时（缺 DLL、被杀软拦截、参数过旧）真正的原因只在它的 stderr 上，
// 而连接 IPC 失败本身只会说「找不到管道」，用户和我们都无从下手。
func mpvLoadError(err error, stderr string) error {
	// trim 一次：调用方可能直接传入未经处理的原始输出。
	if stderr = strings.TrimSpace(stderr); stderr == "" {
		return err
	}
	return fmt.Errorf("%w（mpv 输出: %s）", err, stderr)
}
