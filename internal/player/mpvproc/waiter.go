package mpvproc

import "os/exec"

// waiter 只调用一次 exec.Cmd.Wait，并把结果广播给所有等待者。
//
// exec.Cmd.Wait 不允许被调用两次，而 Load 失败清理、命令应答超时、Close
// 三处都需要给子进程收尸，所以统一走这里，顺带提供一个「进程是否已退出」
// 的非阻塞信号，供等待 IPC 管道时监视。
type waiter struct {
	done chan struct{}
	err  error
}

// startWaiter 起一个 goroutine 收尸。err 在 close(done) 之前写入，因此所有
// <-done 之后读 err 的调用者都有正确的 happens-before。
func startWaiter(cmd *exec.Cmd) *waiter {
	w := &waiter{done: make(chan struct{})}
	go func() {
		w.err = cmd.Wait()
		close(w.done)
	}()
	return w
}

// exited 返回进程退出后关闭的通道，供 select 非阻塞探测。w 为 nil 时返回
// nil 通道（select 中永不就绪），调用方无需判空。
func (w *waiter) exited() <-chan struct{} {
	if w == nil {
		return nil
	}
	return w.done
}

// wait 阻塞到进程退出并返回 Wait 的结果，可安全重复调用。
// w 为 nil 表示没有在跑的子进程，直接返回。
func (w *waiter) wait() error {
	if w == nil {
		return nil
	}
	<-w.done
	return w.err
}
