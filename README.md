# Docker Images Pusher

使用Github Action将国外的Docker镜像转存到阿里云私有仓库，供国内服务器使用，免费易用&lt;br&gt;
- 支持DockerHub, gcr.io, k8s.io, ghcr.io等任意仓库&lt;br&gt;
- 使用skopeo直接在仓库间复制，无需下载到本地磁盘&lt;br&gt;
- 只复制amd64架构，节省空间和时间&lt;br&gt;
- 支持邮件通知同步结果&lt;br&gt;
- 使用阿里云的官方线路，速度快&lt;br&gt;

视频教程：https://www.bilibili.com/video/BV1Zn4y19743/

作者：**[技术爬爬虾](https://github.com/tech-shrimp/me)**&lt;br&gt;
B站，抖音，Youtube全网同名，转载请注明作者&lt;br&gt;
基于他的项目进行了修改&lt;br&gt;

## 功能特性

- ✅ 使用skopeo直接复制，不占用Runner磁盘空间
- ✅ 只复制amd64/linux架构，适合x86平台
- ✅ 自动检测已存在的镜像，跳过重复同步
- ✅ 自动处理镜像重名问题
- ✅ 支持邮件通知同步结果
- ✅ 失败自动重试3次

## 使用方式


### 配置阿里云
登录阿里云容器镜像服务&lt;br&gt;
https://cr.console.aliyun.com/&lt;br&gt;
启用个人实例，创建一个命名空间（**ALIYUN_NAME_SPACE**）
![](/doc/命名空间.png)

访问凭证–&gt;获取环境变量&lt;br&gt;
用户名（**ALIYUN_REGISTRY_USER**)&lt;br&gt;
密码（**ALIYUN_REGISTRY_PASSWORD**)&lt;br&gt;
仓库地址（**ALIYUN_REGISTRY**）&lt;br&gt;

![](/doc/用户名密码.png)


### Fork本项目
Fork本项目&lt;br&gt;
#### 启动Action
进入您自己的项目，点击Action，启用Github Action功能&lt;br&gt;
#### 配置环境变量
进入Settings-&gt;Secret and variables-&gt;Actions-&gt;New Repository secret
![](doc/配置环境变量.png)
将上一步的**四个值**&lt;br&gt;
ALIYUN_NAME_SPACE,ALIYUN_REGISTRY_USER，ALIYUN_REGISTRY_PASSWORD，ALIYUN_REGISTRY&lt;br&gt;
配置成环境变量

#### 可选：配置邮件通知
如需邮件通知，还需添加以下Secret：&lt;br&gt;
- **EMAIL_USERNAME**: 你的163邮箱（如：xxx@163.com）&lt;br&gt;
- **EMAIL_PASSWORD**: 163邮箱授权码（不是邮箱密码）&lt;br&gt;

### 添加镜像
打开images.txt文件，添加你想要的镜像 
可以加tag，也可以不用(默认latest)&lt;br&gt;
可使用 k8s.gcr.io/kube-state-metrics/kube-state-metrics 格式指定私库&lt;br&gt;
可使用 #开头作为注释&lt;br&gt;
![](doc/images.png)
文件提交后，自动进入Github Action构建

### 使用镜像
回到阿里云，镜像仓库，点击任意镜像，可查看镜像状态。(可以改成公开，拉取镜像免登录)
![](doc/开始使用.png)

在国内服务器pull镜像, 例如：&lt;br&gt;
```
docker pull registry.cn-hangzhou.aliyuncs.com/shrimp-images/alpine
```
registry.cn-hangzhou.aliyuncs.com 即 ALIYUN_REGISTRY(阿里云仓库地址)&lt;br&gt;
shrimp-images 即 ALIYUN_NAME_SPACE(阿里云命名空间)&lt;br&gt;
alpine 即 阿里云中显示的镜像名&lt;br&gt;

### 镜像重名
程序自动判断是否存在名称相同, 但是属于不同命名空间的情况。
如果存在，会把命名空间作为前缀加在镜像名称前。
例如:
```
xhofe/alist
xiaoyaliu/alist
```
![](doc/镜像重名.png)

### 定时执行
修改/.github/workflows/docker.yaml文件
添加 schedule即可定时执行(此处cron使用UTC时区)
![](doc/定时执行.png)
