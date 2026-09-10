package pcsupload

import "time"

// EventType 上传事件类型
type EventType int

const (
	// EventFileStarted 文件开始上传
	EventFileStarted EventType = iota
	// EventFileProgress 文件上传进度
	EventFileProgress
	// EventFileDone 文件上传完成
	EventFileDone
	// EventFileFailed 文件上传失败
	EventFileFailed
)

// Event 上传生命周期事件。
//
// 该结构是 pcsupload 与上层输出层之间的结构化输出接缝: 当 UploadTaskUnit.OnEvent
// 被设置时, 任务单元不再向标准输出打印人类可读文本, 而是通过该回调上报事件。
type Event struct {
	Type EventType

	ID        string // 任务 id (本次上传运行内唯一)
	LocalPath string // 本地文件路径
	SavePath  string // 网盘保存路径

	Size     int64         // 文件总大小 (字节)
	Uploaded int64         // 已上传字节数 (仅进度事件)
	Total    int64         // 总字节数 (仅进度事件)
	Speed    int64         // 上传速度 (字节/秒, 仅进度事件)
	Elapsed  time.Duration // 已耗时 (仅进度事件)

	Skipped bool   // 文件是否被跳过 (已存在等)
	Message string // 结果描述 (失败/跳过时携带)
	Err     error  // 失败原因 (仅失败事件)
	Retries int    // 失败时的重试次数
}
