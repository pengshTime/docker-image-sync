package provider

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type AliyunProvider struct {
	registry   string
	namespace  string
	username   string
	password   string
}

func NewAliyunProvider(registry, namespace, username, password string) *AliyunProvider {
	if registry == "" {
		registry = "registry.cn-hangzhou.aliyuncs.com"
	}
	return &AliyunProvider{
		registry:   registry,
		namespace:  namespace,
		username:   username,
		password:   password,
	}
}

func (p *AliyunProvider) Name() string {
	return "Aliyun ACR"
}

func (p *AliyunProvider) RegistryDomain() string {
	return p.registry
}

func (p *AliyunProvider) SyncImage(ctx context.Context, sourceImage string) (*SyncResult, error) {
	targetImage := p.buildTargetImage(sourceImage)
	result := &SyncResult{
		SourceImage: sourceImage,
		TargetImage: targetImage,
		Success:     false,
	}

	exists, err := p.CheckImageExists(ctx, targetImage)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("failed to check image: %v", err)
		return result, err
	}
	if exists {
		result.Success = true
		result.ErrorMessage = "already exists"
		return result, nil
	}

	if err := p.login(); err != nil {
		result.ErrorMessage = fmt.Sprintf("login failed: %v", err)
		return result, err
	}

	if err := p.skopeoCopy(ctx, sourceImage, targetImage); err != nil {
		result.ErrorMessage = fmt.Sprintf("copy failed: %v", err)
		return result, err
	}

	result.Success = true
	return result, nil
}

func (p *AliyunProvider) CheckImageExists(ctx context.Context, image string) (bool, error) {
	cmd := exec.CommandContext(ctx, "skopeo", "inspect", fmt.Sprintf("docker://%s", image))
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "manifest unknown") || strings.Contains(string(output), "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// buildTargetImage 构建目标镜像地址
// 支持多种镜像格式：
//   - python:3.11-slim -> registry/namespace/python:3.11-slim
//   - nginx:latest -> registry/namespace/nginx:latest
//   - jgraph/drawio:latest -> registry/namespace/drawio:latest (阿里云不需要前缀)
//   - docker.io/library/nginx:latest -> registry/namespace/nginx:latest
func (p *AliyunProvider) buildTargetImage(sourceImage string) string {
	// 移除 digest 部分 (@sha256:...)
	if atIdx := strings.Index(sourceImage, "@"); atIdx != -1 {
		sourceImage = sourceImage[:atIdx]
	}

	// 解析镜像名称和标签
	var imageName, tag string
	if colonIdx := strings.LastIndex(sourceImage, ":"); colonIdx != -1 {
		// 检查是否是端口（如 localhost:5000/image）
		afterColon := sourceImage[colonIdx+1:]
		if !strings.Contains(afterColon, "/") {
			imageName = sourceImage[:colonIdx]
			tag = afterColon
		} else {
			imageName = sourceImage
			tag = "latest"
		}
	} else {
		imageName = sourceImage
		tag = "latest"
	}

	// 分割路径获取镜像名
	parts := strings.Split(imageName, "/")
	namePart := parts[len(parts)-1]

	if tag != "" && tag != "latest" {
		return fmt.Sprintf("%s/%s/%s:%s", p.registry, p.namespace, namePart, tag)
	}
	return fmt.Sprintf("%s/%s/%s", p.registry, p.namespace, namePart)
}

func (p *AliyunProvider) login() error {
	cmd := exec.Command("bash", "-c", fmt.Sprintf("echo '%s' | skopeo login --username '%s' --password-stdin %s", p.password, p.username, p.registry))
	return cmd.Run()
}

func (p *AliyunProvider) skopeoCopy(ctx context.Context, source, target string) error {
	cmd := exec.CommandContext(ctx, "skopeo", "copy", "--override-arch", "amd64", "--override-os", "linux",
		fmt.Sprintf("docker://%s", source), fmt.Sprintf("docker://%s", target))
	return cmd.Run()
}
