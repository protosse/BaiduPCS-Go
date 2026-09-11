package pcscommand

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsfunctions/pcsupload"
)

const (
	// uploadProgressThrottle 上传进度事件的最小输出间隔。
	uploadProgressThrottle = 500 * time.Millisecond
)

var (
	jsonOutputMu sync.Mutex
	jsonOutput   io.Writer = os.Stdout
)

// writeJSONLine 原子地向标准输出写入一行 JSON。
// 上传事件可能来自多个并发 goroutine, 因此需要加锁保证行不交错。
func writeJSONLine(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	jsonOutputMu.Lock()
	defer jsonOutputMu.Unlock()
	jsonOutput.Write(append(data, '\n'))
}

// EmitJSON 向标准输出写入一行 JSON (供 main 包等外部调用)。
func EmitJSON(v interface{}) {
	writeJSONLine(v)
}

// QuotaJSON 配额信息 (login / quota 共用)。
type QuotaJSON struct {
	Total int64   `json:"total"`
	Used  int64   `json:"used"`
	Free  int64   `json:"free"`
	Ratio float64 `json:"ratio"` // 已用空间占比, 0~1
}

// FetchQuota 获取当前用户配额信息。
func FetchQuota() (QuotaJSON, error) {
	quota, used, err := GetBaiduPCS().QuotaInfo()
	if err != nil {
		return QuotaJSON{}, err
	}
	free := quota - used
	var ratio float64
	if quota > 0 {
		ratio = float64(used) / float64(quota)
	}
	return QuotaJSON{
		Total: quota,
		Used:  used,
		Free:  free,
		Ratio: ratio,
	}, nil
}

// LoginJSON login 命令的 JSON 输出。
type LoginJSON struct {
	Type     string     `json:"type"`
	OK       bool       `json:"ok"`
	Username string     `json:"username,omitempty"`
	UID      uint64     `json:"uid,omitempty"`
	Quota    *QuotaJSON `json:"quota,omitempty"`
	Error    string     `json:"error,omitempty"`
}

// QuotaCommandJSON quota 命令的 JSON 输出。
type QuotaCommandJSON struct {
	Type     string     `json:"type"`
	OK       bool       `json:"ok"`
	Username string     `json:"username,omitempty"`
	Quota    *QuotaJSON `json:"quota,omitempty"`
	Error    string     `json:"error,omitempty"`
}

// ShareSetJSON share set 命令的 JSON 输出。
type ShareSetJSON struct {
	Type        string `json:"type"`
	OK          bool   `json:"ok"`
	ShareID     int64  `json:"share_id,omitempty"`
	Link        string `json:"link,omitempty"`
	Pwd         string `json:"pwd,omitempty"`
	LinkWithPwd string `json:"link_with_pwd,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ShareCancelJSON share cancel 命令的 JSON 输出。
type ShareCancelJSON struct {
	Type     string  `json:"type"`
	OK       bool    `json:"ok"`
	ShareIDs []int64 `json:"share_ids,omitempty"`
	Error    string  `json:"error,omitempty"`
}

// ShareItemJSON share list 中的单个分享记录。
type ShareItemJSON struct {
	ShareID         int64  `json:"share_id"`
	Shortlink       string `json:"shortlink,omitempty"`
	Pwd             string `json:"pwd,omitempty"`
	LinkWithPwd     string `json:"link_with_pwd,omitempty"`
	TypicalPath     string `json:"typical_path,omitempty"`
	ExpireType      int    `json:"expire_type"`
	ExpireInSeconds int64  `json:"expire_in_seconds"`
	ViewCount       int    `json:"view_count"`
	Error           string `json:"error,omitempty"` // 取提取码失败时携带
}

// ShareListJSON share list 命令的 JSON 输出。
type ShareListJSON struct {
	Type   string          `json:"type"`
	OK     bool            `json:"ok"`
	Shares []ShareItemJSON `json:"shares"`
	Error  string          `json:"error,omitempty"`
}

// ComposeLinkWithPwd 将分享链接与提取码组合成带 pwd= 的完整链接。
func ComposeLinkWithPwd(link, pwd string) string {
	if pwd == "" {
		return link
	}
	if link == "" {
		return ""
	}
	return link + "?pwd=" + pwd
}

// 上传事件 JSON 结构。字段名即 schema 契约, 不可随意变更。

// UploadStartedJSON 上传开始事件 (整个上传运行仅一次)。
type UploadStartedJSON struct {
	Type       string `json:"type"`
	FileCount  int    `json:"file_count"`
	TargetPath string `json:"target_path"`
}

// UploadFileProgressJSON 单文件进度事件。
type UploadFileProgressJSON struct {
	Type      string  `json:"type"`
	FileID    string  `json:"file_id"`
	LocalPath string  `json:"local_path"`
	SavePath  string  `json:"save_path"`
	Uploaded  int64   `json:"uploaded"`
	Total     int64   `json:"total"`
	Speed     int64   `json:"speed"`
	ElapsedMS int64   `json:"elapsed_ms"`
	Percent   float64 `json:"percent"`
}

// UploadFileDoneJSON 单文件完成事件。
type UploadFileDoneJSON struct {
	Type      string `json:"type"`
	FileID    string `json:"file_id"`
	LocalPath string `json:"local_path"`
	SavePath  string `json:"save_path"`
	Size      int64  `json:"size"`
	ElapsedMS int64  `json:"elapsed_ms"`
	Skipped   bool   `json:"skipped"`
}

// UploadFileFailedJSON 单文件失败事件。
type UploadFileFailedJSON struct {
	Type      string `json:"type"`
	FileID    string `json:"file_id"`
	LocalPath string `json:"local_path"`
	SavePath  string `json:"save_path"`
	Error     string `json:"error,omitempty"`
	Message   string `json:"message,omitempty"`
	Retries   int    `json:"retries"`
}

// UploadCompleteJSON 上传完成事件 (整个上传运行仅一次)。
type UploadCompleteJSON struct {
	Type      string `json:"type"`
	OK        bool   `json:"ok"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
	Skipped   int    `json:"skipped"`
	TotalSize int64  `json:"total_size"`
	ElapsedMS int64  `json:"elapsed_ms"`
	Error     string `json:"error,omitempty"`
}

// uploadJSONEmitter 将 pcsupload.Event 转为 JSON-lines 输出, 并做进度节流。
type uploadJSONEmitter struct {
	startedAt    time.Time
	fileStart    map[string]time.Time
	lastProgress map[string]time.Time
	succeeded    int
	skipped      int
	failed       int
	mu           sync.Mutex
}

func newUploadJSONEmitter() *uploadJSONEmitter {
	return &uploadJSONEmitter{
		startedAt:    time.Now(),
		fileStart:    make(map[string]time.Time),
		lastProgress: make(map[string]time.Time),
	}
}

func (e *uploadJSONEmitter) started(fileCount int, targetPath string) {
	writeJSONLine(UploadStartedJSON{
		Type:       "started",
		FileCount:  fileCount,
		TargetPath: targetPath,
	})
}

func (e *uploadJSONEmitter) complete(totalSize int64) {
	e.mu.Lock()
	succeeded, failed, skipped := e.succeeded, e.failed, e.skipped
	e.mu.Unlock()
	writeJSONLine(UploadCompleteJSON{
		Type:      "complete",
		OK:        failed == 0,
		Succeeded: succeeded,
		Failed:    failed,
		Skipped:   skipped,
		TotalSize: totalSize,
		ElapsedMS: time.Since(e.startedAt).Milliseconds(),
	})
}

// completeError 在尚未开始时即失败的情况下输出一个带错误信息的 complete 事件。
func (e *uploadJSONEmitter) completeError(err error) {
	writeJSONLine(UploadCompleteJSON{
		Type:      "complete",
		OK:        false,
		ElapsedMS: time.Since(e.startedAt).Milliseconds(),
		Error:     err.Error(),
	})
}

func (e *uploadJSONEmitter) onEvent(ev pcsupload.Event) {
	switch ev.Type {
	case pcsupload.EventFileStarted:
		e.mu.Lock()
		e.fileStart[ev.ID] = time.Now()
		e.mu.Unlock()

	case pcsupload.EventFileProgress:
		now := time.Now()
		e.mu.Lock()
		if last, ok := e.lastProgress[ev.ID]; ok && now.Sub(last) < uploadProgressThrottle {
			e.mu.Unlock()
			return
		}
		e.lastProgress[ev.ID] = now
		e.mu.Unlock()

		writeJSONLine(UploadFileProgressJSON{
			Type:      "file_progress",
			FileID:    ev.ID,
			LocalPath: ev.LocalPath,
			SavePath:  ev.SavePath,
			Uploaded:  ev.Uploaded,
			Total:     ev.Total,
			Speed:     ev.Speed,
			ElapsedMS: ev.Elapsed.Milliseconds(),
			Percent:   percentOf(ev.Uploaded, ev.Total),
		})

	case pcsupload.EventFileDone:
		e.mu.Lock()
		if ev.Skipped {
			e.skipped++
		} else {
			e.succeeded++
		}
		e.mu.Unlock()

		writeJSONLine(UploadFileDoneJSON{
			Type:      "file_done",
			FileID:    ev.ID,
			LocalPath: ev.LocalPath,
			SavePath:  ev.SavePath,
			Size:      ev.Size,
			ElapsedMS: e.elapsedFor(ev.ID),
			Skipped:   ev.Skipped,
		})

	case pcsupload.EventFileFailed:
		e.mu.Lock()
		e.failed++
		e.mu.Unlock()

		errStr := ""
		if ev.Err != nil {
			errStr = ev.Err.Error()
		}
		writeJSONLine(UploadFileFailedJSON{
			Type:      "file_failed",
			FileID:    ev.ID,
			LocalPath: ev.LocalPath,
			SavePath:  ev.SavePath,
			Error:     errStr,
			Message:   ev.Message,
			Retries:   ev.Retries,
		})
	}
}

func (e *uploadJSONEmitter) elapsedFor(id string) int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if start, ok := e.fileStart[id]; ok {
		return time.Since(start).Milliseconds()
	}
	return 0
}

func percentOf(uploaded, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(uploaded) / float64(total) * 100
}
