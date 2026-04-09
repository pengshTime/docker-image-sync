package provider

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ParsedImage 表示解析后的镜像信息
type ParsedImage struct {
	Registry  string // 如 docker.io
	Namespace string // 如 library 或 jgraph
	Name      string // 镜像名
	Tag       string // 标签
}

// ParseImage 解析已标准化的镜像地址
// 输入格式: docker.io/library/nginx:latest 或 docker.io/jgraph/drawio:latest
func ParseImage(sourceImage string) ParsedImage {
	// 移除 digest 部分 (@sha256:...)
	if atIdx := strings.Index(sourceImage, "@"); atIdx != -1 {
		sourceImage = sourceImage[:atIdx]
	}

	// 解析镜像名称和标签
	var imageRef, tag string
	if colonIdx := strings.LastIndex(sourceImage, ":"); colonIdx != -1 {
		// 确保 : 不是端口的一部分
		afterColon := sourceImage[colonIdx+1:]
		if !strings.Contains(afterColon, "/") {
			imageRef = sourceImage[:colonIdx]
			tag = afterColon
		} else {
			imageRef = sourceImage
			tag = "latest"
		}
	} else {
		imageRef = sourceImage
		tag = "latest"
	}

	// 分割路径
	parts := strings.Split(imageRef, "/")
	
	result := ParsedImage{
		Tag: tag,
	}

	switch len(parts) {
	case 1:
		// 只有镜像名
		result.Name = parts[0]
	case 2:
		// registry/name 或 namespace/name
		if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
			result.Registry = parts[0]
			result.Name = parts[1]
		} else {
			result.Namespace = parts[0]
			result.Name = parts[1]
		}
	default:
		// registry/namespace/name/.../name
		result.Registry = parts[0]
		result.Name = parts[len(parts)-1]
		if len(parts) > 2 {
			result.Namespace = strings.Join(parts[1:len(parts)-1], "_")
		}
	}

	return result
}

// sanitizeImageName 清理镜像名，替换特殊字符
// 华为云/腾讯云对镜像名有严格要求，需要替换 / 和 .
func sanitizeImageName(name string) string {
	// 替换 / 为 _
	name = strings.ReplaceAll(name, "/", "_")
	// 替换 . 为 _
	name = strings.ReplaceAll(name, ".", "_")
	// 替换其他可能不支持的字符
	name = strings.ReplaceAll(name, "-", "_")
	return name
}

// BuildTargetImage 构建目标镜像地址
// 华为云/腾讯云格式: registry/namespace/prefix_name:tag
// 阿里云格式: registry/namespace/name:tag
func BuildTargetImage(registry, namespace string, img ParsedImage, usePrefix bool) string {
	var targetImageName string
	
	// 清理镜像名中的特殊字符
	sanitizedName := sanitizeImageName(img.Name)
	
	if usePrefix && img.Namespace != "" {
		// 清理 namespace 中的特殊字符
		sanitizedNamespace := sanitizeImageName(img.Namespace)
		targetImageName = sanitizedNamespace + "_" + sanitizedName
	} else {
		targetImageName = sanitizedName
	}

	if img.Tag != "" && img.Tag != "latest" {
		return fmt.Sprintf("%s/%s/%s:%s", registry, namespace, targetImageName, img.Tag)
	}
	return fmt.Sprintf("%s/%s/%s", registry, namespace, targetImageName)
}

// checkImageExists 检查镜像是否已存在
// 返回: (是否存在, 错误)
// 如果是权限错误或网络错误，返回 error 终止程序
// 如果是镜像不存在，返回 (false, nil) 继续同步
func checkImageExists(ctx context.Context, image string) (bool, error) {
	cmd := exec.CommandContext(ctx, "skopeo", "inspect", fmt.Sprintf("docker://%s", image))
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		
		// 镜像确实不存在的情况
		if strings.Contains(outputStr, "manifest unknown") ||
			strings.Contains(outputStr, "404") ||
			strings.Contains(outputStr, "not found") ||
			strings.Contains(outputStr, "name unknown") {
			return false, nil
		}
		
		// 权限错误 - 终止程序
		if strings.Contains(outputStr, "401") ||
			strings.Contains(outputStr, "Unauthorized") ||
			strings.Contains(outputStr, "authentication required") {
			return false, fmt.Errorf("unauthorized: check your registry credentials")
		}
		
		// 网络超时错误 - 终止程序
		if strings.Contains(outputStr, "timeout") ||
			strings.Contains(outputStr, "deadline exceeded") ||
			strings.Contains(outputStr, "no such host") ||
			strings.Contains(outputStr, "connection refused") {
			return false, fmt.Errorf("network error: %v", err)
		}
		
		// 其他错误也终止程序
		return false, fmt.Errorf("skopeo inspect failed: %v, output: %s", err, outputStr)
	}
	return true, nil
}

// dockerLogin 执行 docker login
func dockerLogin(registry, username, password string) error {
	cmd := exec.Command("bash", "-c",
		fmt.Sprintf("echo '%s' | skopeo login --username '%s' --password-stdin %s",
			password, username, registry))
	return cmd.Run()
}

// skopeoCopy 复制镜像
func skopeoCopy(ctx context.Context, source, target string) error {
	cmd := exec.CommandContext(ctx, "skopeo", "copy",
		"--override-arch", "amd64",
		"--override-os", "linux",
		"--src-tls-verify=true",
		"--dest-tls-verify=true",
		fmt.Sprintf("docker://%s", source),
		fmt.Sprintf("docker://%s", target))
	return cmd.Run()
}
