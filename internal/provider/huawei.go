package provider

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type HuaweiProvider struct {
	registry   string
	namespace  string
	username   string
	password   string
}

func NewHuaweiProvider(registry, namespace, username, password string) *HuaweiProvider {
	if registry == "" {
		registry = "swr.cn-south-1.myhuaweicloud.com"
	}
	return &HuaweiProvider{
		registry:   registry,
		namespace:  namespace,
		username:   username,
		password:   password,
	}
}

func (p *HuaweiProvider) Name() string {
	return "Huawei SWR"
}

func (p *HuaweiProvider) RegistryDomain() string {
	return p.registry
}

func (p *HuaweiProvider) SyncImage(ctx context.Context, sourceImage string) (*SyncResult, error) {
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

func (p *HuaweiProvider) CheckImageExists(ctx context.Context, image string) (bool, error) {
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
//   - jgraph/drawio:latest -> registry/namespace/jgraph_drawio:latest
//   - docker.io/library/nginx:latest -> registry/namespace/nginx:latest
//   - gcr.io/google-containers/pause:3.9 -> registry/namespace/google-containers_pause:3.9
func (p *HuaweiProvider) buildTargetImage(sourceImage string) string {
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

	// 分割路径获取镜像名和命名空间
	parts := strings.Split(imageName, "/")
	var namePart string
	var namespaceParts []string

	if len(parts) == 1 {
		// 只有镜像名，如 "nginx"
		namePart = parts[0]
	} else if len(parts) == 2 {
		// 可能是 "nginx:latest" 被错误分割，或 "jgraph/drawio"
		if strings.Contains(parts[1], ":") {
			// 是 "nginx:latest" 格式，不应该在这里出现
			namePart = parts[0]
		} else {
			// 是 "jgraph/drawio" 格式
			namespaceParts = []string{parts[0]}
			namePart = parts[1]
		}
	} else {
		// 多个部分，如 "docker.io/library/nginx" 或 "gcr.io/google-containers/pause"
		// 取最后一部分作为镜像名，中间部分作为命名空间
		namePart = parts[len(parts)-1]
		namespaceParts = parts[1 : len(parts)-1]
	}

	// 构建目标镜像名
	var targetImageName string
	if len(namespaceParts) > 0 {
		targetImageName = strings.Join(namespaceParts, "_") + "_" + namePart
	} else {
		targetImageName = namePart
	}

	if tag != "" && tag != "latest" {
		return fmt.Sprintf("%s/%s/%s:%s", p.registry, p.namespace, targetImageName, tag)
	}
	return fmt.Sprintf("%s/%s/%s", p.registry, p.namespace, targetImageName)
}

func (p *HuaweiProvider) login() error {
	cmd := exec.Command("bash", "-c", fmt.Sprintf("echo '%s' | skopeo login --username '%s' --password-stdin %s", p.password, p.username, p.registry))
	return cmd.Run()
}

func (p *HuaweiProvider) skopeoCopy(ctx context.Context, source, target string) error {
	cmd := exec.CommandContext(ctx, "skopeo", "copy", "--override-arch", "amd64", "--override-os", "linux",
		fmt.Sprintf("docker://%s", source), fmt.Sprintf("docker://%s", target))
	return cmd.Run()
}
