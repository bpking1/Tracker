# Traker

本地优先、文本优先的个人影视追踪工具。`data/traker.txt` 是唯一事实来源；Web 界面只负责更方便地查看和编辑它。

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

## 验证

```powershell
go test ./...
bun run build
```

后端仅监听回环地址。若改为局域网地址，应先增加身份验证，当前版本未开放跨域访问。
