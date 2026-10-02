package mpvplugin

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// minMPVVersion 是 UnBox 接受的最低 mpv 版本（产品下限，必要时可调整）。
//
// 更老的 mpv 缺少 UnBox 依赖的 IPC 服务等能力，故障表现是「启动后立即退出」，
// 用户完全看不到与版本有关的线索，只会收到一句「连接 mpv IPC 失败」。
// 与其让这种报错反复出现，不如在创建播放器之前就明确告知。
var minMPVVersion = [3]int{0, 28, 0}

// versionProbeTimeout 限制 mpv --version 的耗时，避免损坏的 mpv 卡住启动流程。
const versionProbeTimeout = 5 * time.Second

// mpvVersionPattern 匹配 `mpv --version` 输出开头的版本号，形如 "mpv 0.41.0"、
// "mpv v0.37.0-..."、"mpv 0.35.1+git.abc"。刻意不加 (?m)：只认输出最开头的
// 那一行，避免正文里出现的其他版本号（依赖库等）被误当成 mpv 自身版本。
var mpvVersionPattern = regexp.MustCompile(`^\s*mpv\s+v?(\d+)\.(\d+)(?:\.(\d+))?`)

// parseMPVVersion 解析版本号；认不出来时 ok=false，交由调用方按「无法判定」处理。
func parseMPVVersion(out string) (version [3]int, ok bool) {
	m := mpvVersionPattern.FindStringSubmatch(out)
	if m == nil {
		return [3]int{}, false
	}
	for i, field := range m[1:] {
		if field == "" {
			continue // 形如 "mpv 0.33" 的两位版本号，patch 位保持 0
		}
		n, err := strconv.Atoi(field)
		if err != nil {
			return [3]int{}, false
		}
		version[i] = n
	}
	return version, true
}

// versionBelow 判断 a 是否低于 b。
func versionBelow(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func formatVersion(v [3]int) string {
	return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2])
}

// checkVersion 运行一次 `mpv --version`，确认可执行文件真的能跑起来并检查版本下限。
//
// 这是唯一能提前发现「mpv.exe 存在、进程也建得起来，但一跑就崩」的手段——
// 典型成因是随便携版分发的 DLL 缺失或被杀软拦截。那种情况下 IPC 管道永远不会
// 出现，等到连接超时才失败，用户只能看到「找不到管道」。
func (m *Manager) checkVersion(ctx context.Context, path string) error {
	probeCtx, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()

	out, err := m.output(probeCtx, path, "--version")
	if err != nil {
		return fmt.Errorf("mpv 无法运行（可能缺少依赖 DLL 或被安全软件拦截）: %w", err)
	}

	version, ok := parseMPVVersion(string(out))
	if ok && versionBelow(version, minMPVVersion) {
		return fmt.Errorf("mpv 版本过低（当前 %s，需要 %s 及以上），请升级 mpv 后重试",
			formatVersion(version), formatVersion(minMPVVersion))
	}
	// 版本号认不出来（分支版本、非英文输出等）时不拦截：下限只是兜底，
	// 优先避免误伤本来能用的 mpv。
	return nil
}
