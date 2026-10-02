package mpvproc

import (
	"errors"
	"strings"
	"testing"
)

func TestStderrTailKeepsOnlyTail(t *testing.T) {
	s := &stderrTail{max: 8}
	if _, err := s.Write([]byte("123456")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := s.Write([]byte("ABCDEF")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := s.String(); got != "56ABCDEF" {
		t.Fatalf("String = %q, want %q", got, "56ABCDEF")
	}
}

// 写少了 io.Copy 会当成错误，进而让 exec 侧把正常输出报成失败。
func TestStderrTailReportsFullWriteLength(t *testing.T) {
	s := &stderrTail{max: 4}
	n, err := s.Write([]byte("abcdef"))
	if err != nil || n != 6 {
		t.Fatalf("Write = (%d,%v), want (6,nil)", n, err)
	}
}

func TestStderrTailEmptyIsEmptyString(t *testing.T) {
	if got := (&stderrTail{max: 8}).String(); got != "" {
		t.Fatalf("String = %q, want 空串", got)
	}
}

func TestMPVLoadErrorCarriesStderr(t *testing.T) {
	cause := errors.New("连接 mpv IPC 失败")
	err := mpvLoadError(cause, "Failed to load libmpv-2.dll")

	if !errors.Is(err, cause) {
		t.Fatal("应包装原错误，保留 errors.Is 判断能力")
	}
	if !strings.Contains(err.Error(), "libmpv-2.dll") {
		t.Fatalf("错误信息应带上 mpv 自己的输出，实际 %q", err.Error())
	}
}

func TestMPVLoadErrorWithoutStderr(t *testing.T) {
	cause := errors.New("连接 mpv IPC 失败")
	got := mpvLoadError(cause, "   ")

	if !errors.Is(got, cause) {
		t.Fatal("应包装原错误")
	}
	if strings.Contains(got.Error(), "mpv 输出") {
		t.Fatalf("无 mpv 输出时不该拼接空原因，实际 %q", got.Error())
	}
}
