# Docker Image Sync

📦 使用 GitHub Actions 将 Docker 镜像从 DockerHub 等仓库同步到阿里云容器镜像服务（ACR）

---

## ✨ 特性

- 🚀 **Skopeo 直接复制** - 不占用 Runner 磁盘空间
- 🎯 **AMD64 架构** - 只同步 x86 平台镜像
- 🔄 **智能去重** - 自动跳过已存在的镜像
- 📧 **邮件通知** - 同步完成自动发送结果邮件
- 💪 **自动重试** - 失败自动重试 3 次
- ⚡ **高速传输** - 使用阿里云官方线路

---

## 📝 使用说明

### 1. 配置阿里云 Secrets

在 GitHub 仓库的 Settings → Secrets and variables → Actions 中添加以下 Secrets：

| Secret Name | 说明 |
|-------------|------|
| `ALIYUN_NAME_SPACE` | 阿里云命名空间 |
| `ALIYUN_REGISTRY_USER` | 阿里云用户名 |
| `ALIYUN_REGISTRY_PASSWORD` | 阿里云密码 |
| `ALIYUN_REGISTRY` | 阿里云仓库地址 |

**可选（邮件通知）：**

| Secret Name | 说明 |
|-------------|------|
| `EMAIL_USERNAME` | 163 邮箱账号 |
| `EMAIL_PASSWORD` | 163 邮箱授权码 |

### 2. 添加镜像列表

编辑 `images.txt`，每行一个镜像：

```txt
jgraph/drawio:latest
azukaar/cosmos-server:latest
corentinth/it-tools:latest
```

### 3. 触发同步

提交 `images.txt` 后，GitHub Actions 自动开始同步。

---

## 📄 感谢

基于 [技术爬爬虾](https://github.com/tech-shrimp/me) 的 [docker_image_pusher](https://github.com/tech-shrimp/docker_image_pusher) 项目进行优化和改进。
