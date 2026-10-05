# Docker 镜像同步工具

📦 使用 GitHub Actions 将 Docker 镜像从 DockerHub 同步到阿里云容器镜像服务（ACR）

---

## ✨ 功能特性

- 🚀 **Skopeo 直接复制** - 无需本地存储，流式传输
- 🎯 **多架构支持** - `skopeo copy --all` 保留 manifest list（AMD64/ARM64 等）
- 🔄 **智能去重** - 跨云自动检测，避免重复拉取
- 🐍 **本地 Python 管理** - 拉取/提交镜像列表、触发同步、查看结果，无需打开网页
- 💪 **自动重试** - 网络错误自动重试，最多 3 次
- ⚡ **并发同步** - 支持多个镜像并行处理
- 🎨 **进度显示** - 实时显示同步进度

---

## 📝 使用说明

### 1. 配置 Secrets

在 GitHub 仓库的 Settings → Secrets and variables → Actions 中添加以下机密信息：

**阿里云配置（必需）：**

| 机密名称 | 说明 | 示例 |
|---------|------|------|
| `ALIYUN_NAME_SPACE` | 阿里云命名空间 | `my-namespace` |
| `ALIYUN_REGISTRY_USER` | 阿里云用户名 | `myusername` |
| `ALIYUN_REGISTRY_PASSWORD` | 阿里云密码 | `mypassword` |
| `ALIYUN_REGISTRY` | 阿里云仓库地址 | `registry.cn-hangzhou.aliyuncs.com` |

### 2. 编辑镜像列表

用本地 Python 工具把远端列表拉到本地编辑：

```bash
python scripts/images.py pull          # 远端 images.txt -> 本地
python scripts/images.py pull --force  # 本地与远端不一致时强行覆盖（默认会拒绝，防丢改动）
```

按以下格式添加镜像：

```txt
# Docker镜像列表
# 格式说明：按云商分组，每行一个镜像地址
# 支持的云商: aliyun

[aliyun]
jgraph/drawio:latest
corentinth/it-tools:latest
```

每行一个镜像地址，不带 registry 前缀时默认按 DockerHub 处理（`jgraph/drawio` 等价于
`docker.io/jgraph/drawio`），不带 tag 时默认 `:latest`。注释行以 `#` 开头。

> 推到阿里云时目标仓库名只取镜像名（`.../命名空间/drawio`），会丢掉源命名空间。
> 所以 `a/nginx` 和 `b/nginx` 会落到同一个仓库名上：先同步的成功，后一个会被判定为
> “已存在”而跳过。列表里不要放同名镜像。

### 3. 提交并同步

```bash
python scripts/images.py push          # 提交列表 + 触发同步 + 等待结果
python scripts/images.py push --no-run # 只提交列表，先不同步
python scripts/images.py run           # 列表不变，只重跑一次同步
python scripts/images.py status        # 查看最近几次运行
python scripts/images.py status 1234567890   # 查看指定 run 的明细
python scripts/images.py watch         # 等待正在运行的任务结束
```

`push` 走 GitHub Contents API 提交，不需要本地 git push；凭据直接复用 git 已登录的
GitHub 账号（也可用 `--token` 或环境变量 `GITHUB_TOKEN` 覆盖）。注意凭据需要 `workflow`
权限才能触发 Actions。

同步结果会同时写到 Actions 的 Job summary 页面，`push`/`watch` 会在终端打印每个镜像的
`[SUCCESS] / [SKIP] / [FAIL]` 明细。

> 编辑 `images.txt` 不再自动触发同步（原来的 `push` 触发已移除），统一由
> `python scripts/images.py push` 显式触发，避免保存列表时误跑一次同步。

---

## ⚙️ 高级配置

### 环境变量

在 Settings → Secrets and variables → Actions 的 **Variables** 里配置，workflow 会透传给程序：

| 变量名 | 默认值 | 说明 |
|-------|--------|------|
| `LOG_LEVEL` | `INFO` | 日志级别 (DEBUG/INFO/WARN/ERROR) |
| `SYNC_TIMEOUT` | `900` | 单个镜像同步超时时间（秒），失败重试的等待按 1s/2s/4s… 递增并封顶 30s |
| `MAX_RETRIES` | `3` | 失败重试次数 |
| `CONCURRENCY` | `3` | 并发同步数量 |
| `PREFERRED_ARCH` | 空 | 留空用 `skopeo copy --all --format docker` 保留多架构；设为 `amd64` 等值则只同步该单一架构。**如果 ACR 拒绝多架构镜像（OCI/docker index），把它设为 `amd64` 即可回退老行为，不用改代码** |

其他说明：

- 单个 job 硬超时 120 分钟，同一时间只允许一个同步在跑（后触发的会排队而不是并发）。
- 带 `@sha256:` 的条目会被判定为无效并报错，因为按 digest 锁定会被静默降级成按 tag 同步。
- Actions 运行记录保留最近 5 次（加当前这次），更早的由 workflow 清理。

## 📄 开源协议

本项目基于 MIT 协议开源。

## 🙏 致谢

感谢以下开源项目的启发：
- [docker_image_pusher](https://github.com/tech-shrimp/docker_image_pusher) by 技术爬爬虾
