# BaiduPCS-Go `--json` 输出契约

本文件定义 `internal/pcscommand` 输出层在 `--json` 模式下产生的机器可读输出。
它同时是 Orcust 上传/分享集成端编码所依据的契约; 机器可校验的 JSON Schema 见
[`pcscommand-json.schema.json`](./pcscommand-json.schema.json)。

通用约定:

- 输出为 **JSON-lines**: 每行一个 JSON 对象, 以 `\n` 结尾。上传命令会输出多行
  (事件流), 其余命令输出一行。
- 每个对象都带一个字符串字段 `type`, 标识该对象的种类。
- 除上传事件流外, 每个对象都带布尔字段 `ok` (`true` 表示成功)。
- 错误对象带字符串字段 `error`; 成功对象中该字段省略。
- 字节大小字段 (`total` / `used` / `free` / `uploaded` / `speed` / `size` /
  `total_size`) 单位均为 **字节**。`speed` 单位为字节/秒。
- 时长字段 (`elapsed_ms`) 单位为毫秒。
- `ratio` 为已用空间占比, 取值 `0.0 ~ 1.0`。
- 上传 `file_id` 在一次 `upload` 运行内唯一 (从 `0` 递增的字符串)。

## 1. login

`login -cookies="..."` 默认输出 JSON; 也可显式加 `--json`。

成功:

```json
{"type":"login","ok":true,"username":"example","uid":123456,"quota":{"total":2199023255552,"used":104857600,"free":2198918257152,"ratio":0.0000477}}
```

失败:

```json
{"type":"login","ok":false,"error":"..."}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `type` | string | 固定 `"login"` |
| `ok` | bool | 登录与配额校验是否成功 |
| `username` | string | 百度账号用户名 (成功时) |
| `uid` | uint64 | 百度账号 uid (成功时) |
| `quota` | object | 配额校验结果 (成功时) |
| `quota.total` | int64 | 总空间 (字节) |
| `quota.used` | int64 | 已用空间 (字节) |
| `quota.free` | int64 | 剩余空间 (字节) |
| `quota.ratio` | number | 已用占比 0.0~1.0 |
| `error` | string | 错误信息 (失败时) |

## 2. quota

`quota --json`

成功:

```json
{"type":"quota","ok":true,"username":"example","quota":{"total":2199023255552,"used":104857600,"free":2198918257152,"ratio":0.0000477}}
```

失败:

```json
{"type":"quota","ok":false,"error":"..."}
```

字段同 login 的 `quota` 对象 (顶层另有 `username`)。

## 3. share

### 3.1 share set

`share set --json <path...>`

成功 (必须暴露**提取码** `pwd` 与**带 `pwd=` 的完整链接** `link_with_pwd`):

```json
{"type":"share_set","ok":true,"share_id":1122334455,"link":"https://pan.baidu.com/s/1AbCdEf","pwd":"abcd","link_with_pwd":"https://pan.baidu.com/s/1AbCdEf?pwd=abcd"}
```

失败:

```json
{"type":"share_set","ok":false,"error":"..."}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `type` | string | 固定 `"share_set"` |
| `ok` | bool | 是否成功 |
| `share_id` | int64 | 分享 id |
| `link` | string | 基础分享链接 (不含 pwd) |
| `pwd` | string | 提取码 |
| `link_with_pwd` | string | `link + "?pwd=" + pwd` 组合后的完整链接 |
| `error` | string | 错误信息 (失败时) |

### 3.2 share cancel

`share cancel --json <shareid...>`

```json
{"type":"share_cancel","ok":true,"share_ids":[1122334455]}
```

失败:

```json
{"type":"share_cancel","ok":false,"error":"..."}
```

### 3.3 share list

`share list --json`

成功:

```json
{"type":"share_list","ok":true,"shares":[{"share_id":1122334455,"shortlink":"https://pan.baidu.com/s/1AbCdEf","pwd":"abcd","link_with_pwd":"https://pan.baidu.com/s/1AbCdEf?pwd=abcd","typical_path":"/来自：Orcust/course","expire_type":0,"expire_time":0,"valid":"永久","view_count":3}]}
```

失败:

```json
{"type":"share_list","ok":false,"error":"..."}
```

`shares[]` 元素字段:

| 字段 | 类型 | 说明 |
|---|---|---|
| `share_id` | int64 | 分享 id |
| `shortlink` | string | 短链接 |
| `pwd` | string | 提取码 |
| `link_with_pwd` | string | `shortlink + "?pwd=" + pwd` |
| `typical_path` | string | 特征路径 |
| `expire_type` | int | 过期类型, `-1` 表示已失效 |
| `expire_time` | int64 | 剩余有效秒数, `0` 表示永久 |
| `valid` | string | 过期时间的人类可读描述 (`永久` / `已过期` / 具体时间) |
| `view_count` | int | 浏览次数 |

## 4. upload

`upload --json <本地路径...> <目标目录>` 输出 JSON-lines 事件流。事件种类:

`started` → (每个文件若干 `file_progress`) → (每个文件一个 `file_done` 或
`file_failed`) → `complete`。

### 4.1 started (运行级, 一次)

```json
{"type":"started","file_count":3,"target_path":"/来自：Orcust/course"}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `type` | string | 固定 `"started"` |
| `file_count` | int | 本次上传的文件总数 |
| `target_path` | string | 网盘目标目录 |

### 4.2 file_progress (文件级, 节流 500ms)

```json
{"type":"file_progress","file_id":"0","local_path":"/home/u/a.mp4","save_path":"/来自：Orcust/course/a.mp4","uploaded":52428800,"total":104857600,"speed":1048576,"elapsed_ms":50000,"percent":50.0}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `type` | string | 固定 `"file_progress"` |
| `file_id` | string | 文件任务 id |
| `local_path` | string | 本地路径 |
| `save_path` | string | 网盘保存路径 |
| `uploaded` | int64 | 已上传字节数 |
| `total` | int64 | 总字节数 |
| `speed` | int64 | 速度 (字节/秒) |
| `elapsed_ms` | int64 | 该文件已耗时 (毫秒) |
| `percent` | number | 完成百分比 0.0~100.0 |

### 4.3 file_done (文件级)

```json
{"type":"file_done","file_id":"0","local_path":"/home/u/a.mp4","save_path":"/来自：Orcust/course/a.mp4","size":104857600,"elapsed_ms":100000,"skipped":false}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `type` | string | 固定 `"file_done"` |
| `file_id` | string | 文件任务 id |
| `local_path` | string | 本地路径 |
| `save_path` | string | 网盘保存路径 |
| `size` | int64 | 文件总字节数 |
| `elapsed_ms` | int64 | 该文件耗时 (毫秒) |
| `skipped` | bool | 是否因已存在等被跳过 |

### 4.4 file_failed (文件级)

```json
{"type":"file_failed","file_id":"1","local_path":"/home/u/b.mp4","save_path":"/来自：Orcust/course/b.mp4","error":"网络错误: ...","message":"上传文件失败","retries":3}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `type` | string | 固定 `"file_failed"` |
| `file_id` | string | 文件任务 id |
| `local_path` | string | 本地路径 |
| `save_path` | string | 网盘保存路径 |
| `error` | string | 底层错误信息 (有则输出) |
| `message` | string | 结果描述 |
| `retries` | int | 已重试次数 |

### 4.5 complete (运行级, 一次, 终止事件)

```json
{"type":"complete","ok":true,"succeeded":2,"failed":0,"skipped":1,"total_size":209715200,"elapsed_ms":150000}
```

运行开始前即失败 (如目标目录非法、本地路径为空) 时:

```json
{"type":"complete","ok":false,"succeeded":0,"failed":0,"skipped":0,"total_size":0,"elapsed_ms":1,"error":"本地路径为空"}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `type` | string | 固定 `"complete"` |
| `ok` | bool | `failed == 0` (且无运行前错误) |
| `succeeded` | int | 成功上传文件数 |
| `failed` | int | 失败文件数 |
| `skipped` | int | 跳过文件数 |
| `total_size` | int64 | 实际上传总字节数 |
| `elapsed_ms` | int64 | 运行总耗时 (毫秒) |
| `error` | string | 运行前错误信息 (有则输出) |
