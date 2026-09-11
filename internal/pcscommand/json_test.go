package pcscommand

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsfunctions/pcsupload"
)

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func captureJSON(t *testing.T, fn func()) string {
	t.Helper()
	jsonOutputMu.Lock()
	old := jsonOutput
	var buf bytes.Buffer
	jsonOutput = &buf
	jsonOutputMu.Unlock()
	defer func() {
		jsonOutputMu.Lock()
		jsonOutput = old
		jsonOutputMu.Unlock()
	}()
	fn()
	return buf.String()
}

func decodeLines(t *testing.T, s string) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	for _, line := range bytes.Split([]byte(s), []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("invalid JSON line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestComposeLinkWithPwd(t *testing.T) {
	cases := []struct {
		link, pwd, want string
	}{
		{"https://pan.baidu.com/s/1AbC", "abcd", "https://pan.baidu.com/s/1AbC?pwd=abcd"},
		{"https://pan.baidu.com/s/1AbC", "", "https://pan.baidu.com/s/1AbC"},
		{"", "abcd", ""},
	}
	for _, c := range cases {
		if got := ComposeLinkWithPwd(c.link, c.pwd); got != c.want {
			t.Fatalf("ComposeLinkWithPwd(%q, %q) = %q, want %q", c.link, c.pwd, got, c.want)
		}
	}
}

func TestPercentOf(t *testing.T) {
	cases := []struct {
		up, total int64
		want      float64
	}{
		{0, 0, 0},
		{50, 100, 50},
		{1, 3, 100.0 / 3.0},
		{100, 0, 0},
	}
	for _, c := range cases {
		got := percentOf(c.up, c.total)
		diff := got - c.want
		if diff < 0 {
			diff = -diff
		}
		if diff > 1e-9 {
			t.Fatalf("percentOf(%d, %d) = %v, want %v", c.up, c.total, got, c.want)
		}
	}
}

func TestSingleShotJSONShapes(t *testing.T) {
	quota := QuotaJSON{Total: 100, Used: 30, Free: 70, Ratio: 0.3}

	login := captureJSON(t, func() {
		EmitJSON(LoginJSON{Type: "login", OK: true, Username: "u", UID: 42, Quota: &quota})
	})
	lines := decodeLines(t, login)
	if len(lines) != 1 || lines[0]["type"] != "login" || lines[0]["uid"].(float64) != 42 {
		t.Fatalf("unexpected login JSON: %s", login)
	}
	if _, ok := lines[0]["quota"].(map[string]interface{}); !ok {
		t.Fatalf("login quota should be an object: %s", login)
	}

	quotaOut := captureJSON(t, func() {
		EmitJSON(QuotaCommandJSON{Type: "quota", OK: true, Username: "u", Quota: &quota})
	})
	lines = decodeLines(t, quotaOut)
	if len(lines) != 1 || lines[0]["type"] != "quota" || lines[0]["ok"] != true {
		t.Fatalf("unexpected quota JSON: %s", quotaOut)
	}

	set := captureJSON(t, func() {
		EmitJSON(ShareSetJSON{
			Type: "share_set", OK: true, ShareID: 7,
			Link: "https://pan.baidu.com/s/1AbC", Pwd: "abcd",
			LinkWithPwd: "https://pan.baidu.com/s/1AbC?pwd=abcd",
		})
	})
	lines = decodeLines(t, set)
	if lines[0]["pwd"] != "abcd" || lines[0]["link_with_pwd"] != "https://pan.baidu.com/s/1AbC?pwd=abcd" {
		t.Fatalf("share_set must expose pwd and composed link: %s", set)
	}
}

func TestUploadEmitterEventStream(t *testing.T) {
	em := newUploadJSONEmitter()

	out := captureJSON(t, func() {
		em.started(2, "/target")
		em.onEvent(pcsupload.Event{Type: pcsupload.EventFileStarted, ID: "0", LocalPath: "/a", SavePath: "/target/a", Size: 10})
		em.onEvent(pcsupload.Event{Type: pcsupload.EventFileProgress, ID: "0", LocalPath: "/a", SavePath: "/target/a", Uploaded: 5, Total: 10, Speed: 1, Elapsed: time.Second})
		em.onEvent(pcsupload.Event{Type: pcsupload.EventFileDone, ID: "0", LocalPath: "/a", SavePath: "/target/a", Size: 10})
		em.onEvent(pcsupload.Event{Type: pcsupload.EventFileFailed, ID: "1", LocalPath: "/b", SavePath: "/target/b", Err: &testError{"boom"}})
		em.complete(10)
	})

	lines := decodeLines(t, out)
	if len(lines) != 5 {
		t.Fatalf("expected 5 events, got %d: %s", len(lines), out)
	}
	if lines[0]["type"] != "started" || lines[len(lines)-1]["type"] != "complete" {
		t.Fatalf("unexpected first/last events: %s", out)
	}
	wantTypes := []string{"started", "file_progress", "file_done", "file_failed", "complete"}
	seen := map[string]bool{}
	for _, l := range lines {
		seen[l["type"].(string)] = true
	}
	for _, wt := range wantTypes {
		if !seen[wt] {
			t.Fatalf("missing event type %q in stream: %s", wt, out)
		}
	}
}

func TestUploadEmitterProgressThrottle(t *testing.T) {
	em := newUploadJSONEmitter()

	out := captureJSON(t, func() {
		em.onEvent(pcsupload.Event{Type: pcsupload.EventFileProgress, ID: "0", LocalPath: "/a", SavePath: "/t/a", Uploaded: 1, Total: 10})
		em.onEvent(pcsupload.Event{Type: pcsupload.EventFileProgress, ID: "0", LocalPath: "/a", SavePath: "/t/a", Uploaded: 2, Total: 10})
	})
	if n := len(decodeLines(t, out)); n != 1 {
		t.Fatalf("progress should be throttled to one event, got %d: %s", n, out)
	}

	// 超过节流窗口后应再次输出
	time.Sleep(uploadProgressThrottle + 100*time.Millisecond)
	out2 := captureJSON(t, func() {
		em.onEvent(pcsupload.Event{Type: pcsupload.EventFileProgress, ID: "0", LocalPath: "/a", SavePath: "/t/a", Uploaded: 3, Total: 10})
	})
	if n := len(decodeLines(t, out2)); n != 1 {
		t.Fatalf("progress after throttle window should emit again, got %d: %s", n, out2)
	}
}
