package mpvproc

import (
	"errors"
	"io"
	"testing"
	"time"
)

// stubConn 满足 io.ReadWriteCloser，仅用于验证连接成功这一分支。
type stubConn struct{ closed bool }

func (c *stubConn) Read([]byte) (int, error)    { return 0, io.EOF }
func (c *stubConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *stubConn) Close() error                { c.closed = true; return nil }

// errDial 是伪造的连接失败，替代各平台真实的管道/socket 错误。
var errDial = errors.New("找不到管道")

func alwaysFailDial(string) (io.ReadWriteCloser, error) { return nil, errDial }

// 关键用例：mpv 启动即崩时管道永远不会出现，不能空等满超时——
// 那样真正的失败原因会被拖到 5 秒后才暴露，且只剩一句「找不到管道」。
func TestWaitForIPCStopsEarlyWhenProcessExits(t *testing.T) {
	w := &waiter{done: make(chan struct{})}
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(w.done)
	}()

	start := time.Now()
	_, err := waitForIPC(alwaysFailDial, "unused", 30*time.Second, w)
	elapsed := time.Since(start)

	if !errors.Is(err, errMPVExitedEarly) {
		t.Fatalf("err = %v, want 包装 errMPVExitedEarly", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("进程已退出却等了 %v，应立即返回", elapsed)
	}
}

func TestWaitForIPCRetriesUntilConnected(t *testing.T) {
	var attempts int
	dial := func(string) (io.ReadWriteCloser, error) {
		attempts++
		if attempts < 3 {
			return nil, errDial
		}
		return &stubConn{}, nil
	}

	conn, err := waitForIPC(dial, "unused", 5*time.Second, &waiter{done: make(chan struct{})})
	if err != nil {
		t.Fatalf("waitForIPC: %v", err)
	}
	if conn == nil {
		t.Fatal("连接成功后应返回 conn")
	}
	if attempts != 3 {
		t.Fatalf("尝试次数 = %d, want 3", attempts)
	}
}

// 进程还活着但一直连不上（例如管道被别的实例占着）：按超时返回最后一次
// 连接错误，不能误报成「启动后立即退出」。
func TestWaitForIPCReportsDialErrorOnTimeout(t *testing.T) {
	_, err := waitForIPC(alwaysFailDial, "unused", 60*time.Millisecond, &waiter{done: make(chan struct{})})

	if !errors.Is(err, errDial) {
		t.Fatalf("err = %v, want 最后一次连接错误", err)
	}
	if errors.Is(err, errMPVExitedEarly) {
		t.Fatal("进程未退出时不应报「启动后立即退出」")
	}
}

// waiter 为 nil 时（无子进程可监视）不应 panic，退化为纯重试。
func TestWaitForIPCToleratesNilWaiter(t *testing.T) {
	conn, err := waitForIPC(
		func(string) (io.ReadWriteCloser, error) { return &stubConn{}, nil },
		"unused", time.Second, nil,
	)
	if err != nil || conn == nil {
		t.Fatalf("waitForIPC = (%v,%v), want 成功连接", conn, err)
	}
}
