package mpvplugin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// missingLookPath 让 Status() 找不到系统 mpv，把测试固定在「预检」这条路径上。
func missingLookPath(string) (string, error) { return "", errors.New("missing") }

func TestParseMPVVersion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want [3]int
		ok   bool
	}{
		{name: "标准首行", in: "mpv 0.41.0\nCopyright © 2000-2025", want: [3]int{0, 41, 0}, ok: true},
		{name: "带 v 前缀", in: "mpv v0.37.0", want: [3]int{0, 37, 0}, ok: true},
		{name: "带 git 后缀", in: "mpv 0.35.1+git.abc", want: [3]int{0, 35, 1}, ok: true},
		{name: "只有两段", in: "mpv 0.33", want: [3]int{0, 33, 0}, ok: true},
		{name: "非首行不匹配", in: "版本信息见 https://mpv.io\nmpv 0.41.0", want: [3]int{}, ok: false},
		{name: "完全不认识", in: "some other player", want: [3]int{}, ok: false},
		{name: "空输出", in: "", want: [3]int{}, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseMPVVersion(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Fatalf("version = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVersionBelow(t *testing.T) {
	tests := []struct {
		a, b [3]int
		want bool
	}{
		{a: [3]int{0, 27, 9}, b: [3]int{0, 28, 0}, want: true},
		{a: [3]int{0, 28, 0}, b: [3]int{0, 28, 0}, want: false},
		{a: [3]int{0, 41, 0}, b: [3]int{0, 28, 0}, want: false},
		{a: [3]int{1, 0, 0}, b: [3]int{0, 28, 0}, want: false},
	}
	for _, tt := range tests {
		if got := versionBelow(tt.a, tt.b); got != tt.want {
			t.Fatalf("versionBelow(%v,%v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// 缺 DLL 被杀软拦截的 mpv.exe：os.Stat 通过、进程也建得起来，但一跑就失败。
// 这正是「连接 mpv IPC 失败」最常见的成因，必须在这里就给出可读的原因。
func TestCheckVersionReportsUnrunnableExecutable(t *testing.T) {
	m := newManager("windows", t.TempDir(), missingLookPath)
	m.output = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("exit status 3221225781")
	}

	err := m.checkVersion(context.Background(), `C:\app\mpv\mpv.exe`)
	if err == nil {
		t.Fatal("mpv 跑不起来时应报错")
	}
	if !strings.Contains(err.Error(), "mpv 无法运行") {
		t.Fatalf("错误应说明 mpv 跑不起来，实际 %q", err.Error())
	}
}

func TestCheckVersionRejectsTooOld(t *testing.T) {
	m := newManager("windows", t.TempDir(), missingLookPath)
	m.output = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("mpv 0.27.0\n"), nil
	}

	err := m.checkVersion(context.Background(), "mpv.exe")
	if err == nil {
		t.Fatal("低于下限的 mpv 应被拦截")
	}
	if !strings.Contains(err.Error(), "版本过低") || !strings.Contains(err.Error(), "0.27.0") {
		t.Fatalf("错误应说明版本过低并带上当前版本，实际 %q", err.Error())
	}
}

func TestCheckVersionAcceptsSupported(t *testing.T) {
	m := newManager("windows", t.TempDir(), missingLookPath)
	m.output = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("mpv 0.41.0\n"), nil
	}

	if err := m.checkVersion(context.Background(), "mpv.exe"); err != nil {
		t.Fatalf("受支持的 mpv 不应被拦截: %v", err)
	}
}

// 版本号认不出来时不拦截：下限只是兜底，优先避免误伤能用的 mpv（例如分支版本）。
func TestCheckVersionFailsOpenOnUnknownFormat(t *testing.T) {
	m := newManager("windows", t.TempDir(), missingLookPath)
	m.output = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("mpv (unknown build)\n"), nil
	}

	if err := m.checkVersion(context.Background(), "mpv.exe"); err != nil {
		t.Fatalf("无法判定版本时不应拦截: %v", err)
	}
}

func TestCheckVersionPassesVersionFlag(t *testing.T) {
	m := newManager("windows", t.TempDir(), missingLookPath)
	var gotArgs []string
	m.output = func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte("mpv 0.41.0\n"), nil
	}

	if err := m.checkVersion(context.Background(), "mpv.exe"); err != nil {
		t.Fatalf("checkVersion: %v", err)
	}
	if len(gotArgs) != 2 || gotArgs[1] != "--version" {
		t.Fatalf("调用参数 = %v, want [mpv.exe --version]", gotArgs)
	}
}
