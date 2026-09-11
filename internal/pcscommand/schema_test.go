package pcscommand

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsfunctions/pcsupload"
)

// schemaFile 只关心 JSON Schema 里用于逐行校验的最小结构:
// 每个 definition 的 required 列表与 type 的 const 值。
type schemaFile struct {
	Definitions map[string]struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Const string `json:"const"`
		} `json:"properties"`
	} `json:"definitions"`
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

// validateLine 校验一行 JSON 对象: type 必须有对应 definition, 且 required 字段齐全。
func validateLine(t *testing.T, schema *schemaFile, line map[string]interface{}) {
	t.Helper()
	typ, ok := line["type"].(string)
	if !ok {
		t.Fatalf("line missing string 'type': %v", line)
	}
	for _, def := range schema.Definitions {
		if def.Properties["type"].Const == typ {
			for _, req := range def.Required {
				if _, ok := line[req]; !ok {
					t.Fatalf("line type %q missing required field %q: %v", typ, req, line)
				}
			}
			return
		}
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

// TestRecordedSessionMatchesSchema 校验提交的录制会话逐行符合已发布 schema (AC #3)。
func TestRecordedSessionMatchesSchema(t *testing.T) {
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
