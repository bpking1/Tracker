# Traker

本地优先、文本优先的个人影视追踪工具。`data/traker.txt` 是唯一事实来源；Web 界面只负责更方便地查看和编辑它。

界面支持输入标题后回车快速添加，可同时搜索用户标题、TMDB 标准标题、TMDB 原始标题、演员、题材、标签和短评，并能按 TMDB 匹配状态及电影/剧集类型筛选。详情页的演员姓名可以直接点击并跳转到对应搜索结果。点击“批量匹配”时会先补全已有 ID 记录缺失或异常的元数据，再为未匹配记录采用 TMDB 第一条搜索结果。单条快速添加后仍会打开匹配页供用户确认。

单条或批量 TMDB 匹配产生相同 `tm:`/`tv:` ID 时，界面会提示片单中已存在相同记录，但不会阻止保存，以保留重看记录的使用方式。

左侧“观看时间线”根据看过、弃看记录的完成或结束日期自动按年月分组，不增加额外事件数据，也不会修改主文本。

## 开发运行

要求 Go 1.23+ 和 Bun 1.2+。

```powershell
# 终端一：后端
go run ./cmd/traker

# 终端二：前端（自动代理 /api 到 8080）
bun install
bun run dev
```

打开 `http://127.0.0.1:5173`。首次启动后端会自动创建 `data/traker.txt`。

前端纯函数测试可运行：

```powershell
bun run test
```

## 单进程运行

```powershell
bun install
bun run build
go run ./cmd/traker
```

打开 `http://127.0.0.1:8080`。前端构建产物会嵌入 Go 可执行文件。

可通过参数或环境变量调整监听地址和数据文件：

```powershell
go run ./cmd/traker -addr 127.0.0.1:9000 -data D:\Notes\traker.txt
# 或 TRAKER_ADDR、TRAKER_DATA_FILE
```

## 配置 TMDB

在 [TMDB API 设置](https://www.themoviedb.org/settings/api)申请凭证。推荐使用页面中的 **API Read Access Token**，将 `.env.example` 复制为不会提交到 Git 的 `.env`：

```dotenv
TMDB_API_TOKEN=粘贴你的_API_Read_Access_Token
```

也可以使用传统 API Key：

```dotenv
TMDB_API_KEY=粘贴你的_API_Key
```

修改 `.env` 后需要重启 Go 服务。系统环境变量优先于 `.env`；PowerShell 临时设置方式如下：

```powershell
$env:TMDB_API_TOKEN="你的令牌"
go run ./cmd/traker
```

在记录菜单中选择“匹配 TMDB”，确认结果后会自动把 `tm:<id>` 或 `tv:<id>` 写入 `traker.txt`。标准标题、简介、演员和公共评分保存在 `data/cache/metadata.json`，海报保存在 `data/cache/images/`；这些缓存可以随时删除并重新抓取。

## 配置媒体库播放

使用 `EMBY_SERVERS` JSON 数组配置一个或多个 Emby 服务，数组顺序就是查询顺序：

```dotenv
EMBY_SERVERS=[{"name":"家中","url":"http://127.0.0.1:8096","apiKey":"你的_Emby_API_Key"},{"name":"备用","url":"https://emby.example.com","apiKey":"另一个_Emby_API_Key"}]
```

每项的 `name` 可以省略，`url` 可以带或不带末尾的 `/emby`。配置只在后端读取，不会显示在设置页。修改 `.env` 后需要重启 Go 服务。

Plex 使用独立的 `PLEX_SERVERS` JSON 数组配置：

```dotenv
PLEX_SERVERS=[{"name":"家中 Plex","url":"http://127.0.0.1:32400","token":"你的_Plex_Token"},{"name":"备用 Plex","url":"https://plex.example.com","token":"另一个_Plex_Token"}]
```

Plex 的 `name` 同样可以省略。使用 `PLAYBACK_PROVIDER_ORDER` 配置 Emby 和 Plex 的查询优先级，默认值为 `emby,plex`；要优先查询 Plex，可以这样配置：

```dotenv
PLAYBACK_PROVIDER_ORDER=plex,emby
```

该配置必须同时且各包含一次 `emby`、`plex`，只影响电影播放来源。来源内部仍然按照对应服务器数组的顺序查询，命中第一条结果后停止。Plex 不提供按 TMDB ID 直接搜索媒体库的接口；Traker 使用 TMDB 标准名称和原始名称搜索候选，再读取候选的外部 GUID，通过 `tmdb://<id>` 精确校验，避免仅凭同名条目播放错误内容。配置只在后端读取，不会写入前端、数据文件或元数据缓存。

包含 `tm:` ID 的电影会在详情页显示“播放”按钮；前端使用 ArtPlayer 在站内播放媒体库返回的静态视频地址。包含 `tv:` ID 的剧集会显示“选择剧集”按钮，剧集播放只查询 `PLEX_SERVERS`，不使用 Emby，也不受 `PLAYBACK_PROVIDER_ORDER` 影响。Traker 先按名称匹配 Plex show 并用 TMDB GUID 校验，再读取 Plex 的季和集目录；用户选择具体一集后，后端校验 episode 属于该 show，并返回第一个媒体 Part 的直连地址。

视频由浏览器原生 `<video>` 直接读取最终媒体地址，Traker 后端不代理视频数据，也不解析媒体容器。电影使用 TMDB ID 记忆播放进度；剧集使用 TMDB ID 加季、集编号组成的独立标识，因此不同集不会共用播放进度。播放器提供画中画、倍速、迷你进度条和全屏控制。实际能否播放取决于浏览器对文件容器和音视频编码的原生支持；MKV 通常不能保证播放。读取或解码失败时，错误会显示在播放器下方。

播放器标题栏提供本地字幕入口，支持 UTF-8 编码的 `.srt`、`.vtt` 和 `.ass` 文件；字幕只在当前播放窗口中使用，不会上传或写入数据文件。加载后可以关闭或重新选择字幕，播放器设置菜单可调整字幕时间偏移。ArtPlayer 会将 SRT 和 ASS 转换为 VTT，因此 ASS 的复杂字体、定位、动画和特效不会完整保留。

电影和剧集选集界面的“播放”下拉菜单都可以通过 `potplayer://` 或 [`mpv-handler://`](https://github.com/akiirui/mpv-handler) 协议把同一地址交给外部播放器，也可以复制播放地址。对应播放器及协议处理程序需要预先在当前设备注册；无法注册协议时仍可复制地址后手动打开。记录中的 `S03E02` 进度只用于打开选集界面时默认定位，不会自动推断或播放下一集。

剧集目录在当前浏览器页面会话中按 TMDB ID 缓存，重复打开同一剧集不会再次查询 Plex；同时发起的重复请求也会合并。Plex 新增剧集后可使用选集界面中的刷新按钮强制重新读取目录。失败的请求不会保留在缓存中，下一次打开会自动重试。

## 验证

```powershell
go test ./...
bun run test
bun run build
```

后端仅监听回环地址。若改为局域网地址，应先增加身份验证，当前版本未开放跨域访问。
