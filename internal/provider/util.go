package provider

import (
	"context"
	"fmt"
	"os"
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

// sanitizeImageName 清理镜像名，替换仓库侧可能不接受的特殊字符
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
// usePrefix=false: registry/namespace/name:tag（源命名空间被丢弃，同名镜像会撞车）
// usePrefix=true:  registry/namespace/源命名空间_name:tag
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

// skopeoCopyArgs 组装 skopeo copy 参数
// PREFERRED_ARCH 为空时用 --all 保留 manifest list，并把 media type 转成 docker 格式，
// 避免部分镜像仓库（如 ACR 个人版）拒绝 OCI index；非空时只复制指定单一架构。
func skopeoCopyArgs(source, target string) []string {
	args := []string{"copy", "--src-tls-verify=true", "--dest-tls-verify=true"}
	if arch := os.Getenv("PREFERRED_ARCH"); arch != "" {
		args = append(args, "--override-arch", arch, "--override-os", "linux")
	} else {
		args = append(args, "--all", "--format", "docker")
	}
	return append(args, fmt.Sprintf("docker://%s", source), fmt.Sprintf("docker://%s", target))
}

func skopeoCopy(ctx context.Context, source, target string) error {
	output, err := exec.CommandContext(ctx, "skopeo", skopeoCopyArgs(source, target)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, tail(output))
	}
	return nil
}

// dockerLogin 使用 skopeo login，密码通过 stdin 传入，避免出现在命令行参数和进程列表中
func dockerLogin(registry, username, password string) error {
	cmd := exec.Command("skopeo", "login", "--username", username, "--password-stdin", registry)
	cmd.Stdin = strings.NewReader(password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, tail(output))
	}
	return nil
}

// tail 截断过长的命令输出，保留末尾的错误信息
func tail(output []byte) string {
	const maxLen = 512
	s := strings.TrimSpace(string(output))
	if len(s) <= maxLen {
		return s
	}
	return "..." + s[len(s)-maxLen:]
}
