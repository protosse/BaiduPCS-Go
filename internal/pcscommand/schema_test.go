package pcscommand

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsfunctions/pcsupload"
)

// schemaProperty 只关心校验所需的 property 元数据。
type schemaProperty struct {
	Const string `json:"const"`
	Type  string `json:"type"`
	Ref   string `json:"$ref"`
}

type schemaDef struct {
	Required   []string                  `json:"required"`
	Properties map[string]schemaProperty `json:"properties"`
}

// schemaFile 只关心 JSON Schema 里用于逐行校验的最小结构。
type schemaFile struct {
	Definitions map[string]schemaDef `json:"definitions"`
}

func loadSchema(t *testing.T) *schemaFile {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "pcscommand-json.schema.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", path, err)
	}
	var schema schemaFile
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	if len(schema.Definitions) == 0 {
		t.Fatalf("schema has no definitions")
	}
	return &schema
}

// jsonTypeOf 返回 Go json.Unmarshal 结果的 JSON Schema 类型名。
func jsonTypeOf(v interface{}) string {
	switch v.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	}
	return "unknown"
}

// typeMatches 判断值是否匹配 schema 声明的类型 (integer 要求整数值)。
func typeMatches(v interface{}, want string) bool {
	switch want {
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == math.Trunc(f)
	case "number":
		_, ok := v.(float64)
		return ok
	case "array":
		_, ok := v.([]interface{})
		return ok
	case "object":
		_, ok := v.(map[string]interface{})
		return ok
	}
	return true // 未知类型不校验
}

// validateLine 校验一行 JSON 对象: type 有对应 definition, required 字段齐全,
// 无未声明字段 (additionalProperties:false), 且声明类型的字段类型匹配。
func validateLine(t *testing.T, schema *schemaFile, line map[string]interface{}) {
	t.Helper()
	typ, ok := line["type"].(string)
	if !ok {
		t.Fatalf("line missing string 'type': %v", line)
	}
	for _, def := range schema.Definitions {
		if def.Properties["type"].Const != typ {
			continue
		}
		for _, req := range def.Required {
			if _, ok := line[req]; !ok {
				t.Fatalf("line type %q missing required field %q: %v", typ, req, line)
			}
		}
		for k, v := range line {
			prop, declared := def.Properties[k]
			if !declared {
				t.Fatalf("line type %q has undeclared field %q: %v", typ, k, line)
			}
			if prop.Ref != "" || prop.Type == "" {
				continue // $ref 或无类型声明, 跳过类型校验
			}
			if !typeMatches(v, prop.Type) {
				t.Fatalf("line type %q field %q has JSON type %s, schema wants %s", typ, k, jsonTypeOf(v), prop.Type)
			}
		}
		return
	}
	t.Fatalf("no schema definition for type %q", typ)
}

func linesOf(t *testing.T, s string) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("invalid JSON line %q: %v", sc.Text(), err)
		}
		out = append(out, m)
	}
	return out
}

// TestSessionFixtureMatchesSchema 校验提交的会话夹具逐行符合已发布 schema (AC #3)。
// 注意: 该夹具是代表性合成会话 (本机无真实百度凭证), 而非真实网络抓包;
// 用真实 upload --json 的 stdout 覆盖 testdata/upload-session.jsonl 即可转为真机验证。
func TestSessionFixtureMatchesSchema(t *testing.T) {
	schema := loadSchema(t)

	path := filepath.Join("testdata", "upload-session.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}

	lines := linesOf(t, string(data))
	if len(lines) == 0 {
		t.Fatalf("fixture %s is empty", path)
	}
	for _, line := range lines {
		validateLine(t, schema, line)
	}
}

// TestEmitterOutputMatchesSchema 让真实 emitter 生成一段会话, 逐行校验 (schema 即契约)。
func TestEmitterOutputMatchesSchema(t *testing.T) {
	schema := loadSchema(t)
	em := newUploadJSONEmitter()

	out := captureJSON(t, func() {
		em.started(2, "/target")
		em.onEvent(pcsupload.Event{Type: pcsupload.EventFileStarted, ID: "0", LocalPath: "/a", SavePath: "/t/a", Size: 10})
		em.onEvent(pcsupload.Event{Type: pcsupload.EventFileProgress, ID: "0", LocalPath: "/a", SavePath: "/t/a", Uploaded: 5, Total: 10, Speed: 1, Elapsed: time.Second})
		em.onEvent(pcsupload.Event{Type: pcsupload.EventFileDone, ID: "0", LocalPath: "/a", SavePath: "/t/a", Size: 10})
		em.onEvent(pcsupload.Event{Type: pcsupload.EventFileFailed, ID: "1", LocalPath: "/b", SavePath: "/t/b", Err: &testError{"boom"}})
		em.complete(10)
	})

	for _, line := range linesOf(t, out) {
		validateLine(t, schema, line)
	}
}

// TestSingleShotMatchesSchema 校验 login/quota/share 系列单行输出符合 schema。
func TestSingleShotMatchesSchema(t *testing.T) {
	schema := loadSchema(t)
	quota := QuotaJSON{Total: 100, Used: 30, Free: 70, Ratio: 0.3}

	out := captureJSON(t, func() {
		EmitJSON(LoginJSON{Type: "login", OK: true, Username: "u", UID: 42, Quota: &quota})
		EmitJSON(QuotaCommandJSON{Type: "quota", OK: true, Username: "u", Quota: &quota})
		EmitJSON(ShareSetJSON{Type: "share_set", OK: true, ShareID: 7, Link: "https://pan.baidu.com/s/1AbC", Pwd: "abcd", LinkWithPwd: "https://pan.baidu.com/s/1AbC?pwd=abcd"})
		EmitJSON(ShareCancelJSON{Type: "share_cancel", OK: true, ShareIDs: []int64{1, 2}})
		EmitJSON(ShareListJSON{Type: "share_list", OK: true, Shares: []ShareItemJSON{
			{ShareID: 7, Shortlink: "https://pan.baidu.com/s/1AbC", Pwd: "abcd", LinkWithPwd: "https://pan.baidu.com/s/1AbC?pwd=abcd", TypicalPath: "/p", ExpireType: 0, ExpireInSeconds: 0, ViewCount: 1},
		}})
	})

	for _, line := range linesOf(t, out) {
		validateLine(t, schema, line)
	}
}

// TestRunUploadJSONModeOffline 离线验证 --json 模式: stdout 只承载一行 JSON、
// 失败返回非零错误 (F5), 且重定向在返回后恢复。
func TestRunUploadJSONModeOffline(t *testing.T) {
	// 捕获 JSON (writeJSONLine 写到包级 jsonOutput, 即原始 stdout)
	oldJSON := jsonOutput
	var jsonBuf bytes.Buffer
	jsonOutput = &jsonBuf
	defer func() { jsonOutput = oldJSON }()

	// 捕获 stderr (RunUpload 的 JSON 分支把 os.Stdout 重定向到 os.Stderr)
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		w.Close()
		r.Close()
	}()

	uerr := RunUpload(nil, "/x", &UploadOptions{JSON: true})
	if uerr == nil {
		t.Fatal("expected non-nil error for empty localPaths")
	}
	w.Close()
	_, _ = io.ReadAll(r)

	lines := linesOf(t, jsonBuf.String())
	if len(lines) != 1 {
		t.Fatalf("stdout should hold exactly one JSON line, got %d: %s", len(lines), jsonBuf.String())
	}
	l := lines[0]
	if l["type"] != "complete" || l["ok"] != false {
		t.Fatalf("unexpected JSON: %s", jsonBuf.String())
	}
	if l["error"] == "" || l["error"] == nil {
		t.Fatalf("complete should carry an error: %s", jsonBuf.String())
	}
	if os.Stdout != oldStdout {
		t.Fatal("os.Stdout was not restored after RunUpload")
	}
}
