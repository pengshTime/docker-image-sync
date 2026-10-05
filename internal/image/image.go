package image

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ImageEntry 表示一个镜像条目
type ImageEntry struct {
	Raw       string // 原始输入
	Source    string // 标准化后的源镜像地址
	Alias     string // 别名（如果有）
	Valid     bool   // 是否有效
	ErrorMsg  string // 错误信息（如果无效）
}

// ImageList 表示按提供商分组的镜像列表
type ImageList struct {
	Images map[string][]ImageEntry
}

// LoadFromFile 从文件加载镜像列表
// 支持格式：
//   - [provider] 分组
//   - 镜像地址（支持别名：alias=image:tag）
//   - # 注释
func LoadFromFile(filepath string) (*ImageList, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	images := make(map[string][]ImageEntry)
	var currentSection string

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		
		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 处理分组标记 [provider]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.ToLower(strings.Trim(line, "[]"))
			if _, ok := images[currentSection]; !ok {
				images[currentSection] = []ImageEntry{}
			}
			continue
		}

		// 如果没有分组，使用默认分组
		if currentSection == "" {
			currentSection = "default"
			if _, ok := images[currentSection]; !ok {
				images[currentSection] = []ImageEntry{}
			}
		}

		// 解析镜像条目
		entry := parseImageEntry(line)
		if !entry.Valid {
			entry.ErrorMsg = fmt.Sprintf("line %d: %s", lineNum, entry.ErrorMsg)
		}
		images[currentSection] = append(images[currentSection], entry)
	}

	return &ImageList{Images: images}, scanner.Err()
}

// parseImageEntry 解析单个镜像条目
// 支持格式：
//   - python:3.11-slim
//   - mypython=python:3.11-slim （别名语法）
//   - docker.io/library/nginx:latest
func parseImageEntry(line string) ImageEntry {
	entry := ImageEntry{
		Raw:    line,
		Valid:  true,
	}

	// 检查别名语法 alias=image
	if idx := strings.Index(line, "="); idx > 0 && !strings.Contains(line[:idx], "/") && !strings.Contains(line[:idx], ":") {
		entry.Alias = strings.TrimSpace(line[:idx])
		line = strings.TrimSpace(line[idx+1:])
	}

	// 标准化镜像地址
	source, err := normalizeImageRef(line)
	if err != nil {
		entry.Valid = false
		entry.ErrorMsg = err.Error()
		return entry
	}

	entry.Source = source
	return entry
}

// normalizeImageRef 标准化镜像地址
// 添加默认 registry 和 tag
func normalizeImageRef(image string) (string, error) {
	if image == "" {
		return "", fmt.Errorf("empty image reference")
	}

	// 带 digest 的写法会被静默降级成按 tag 同步，实际拉到的不是锁定的那个版本，直接拒绝
	if atIdx := strings.Index(image, "@"); atIdx != -1 {
		return "", fmt.Errorf("%s uses a digest; pin by tag instead of @sha256:...", image)
	}

	// 检查是否包含 registry（通过判断是否有 "." 或 ":" 在第一个 "/" 之前）
	slashIdx := strings.Index(image, "/")
	colonIdx := strings.Index(image, ":")
	
	hasRegistry := false
	if slashIdx == -1 {
		// 没有 /，可能是 nginx 或 nginx:latest
		hasRegistry = false
	} else if colonIdx != -1 && colonIdx < slashIdx {
		// 第一个 : 在第一个 / 之前，如 localhost:5000/nginx
		hasRegistry = true
	} else if strings.Contains(image[:slashIdx], ".") {
		// 第一个 / 之前包含 .，如 docker.io/nginx
		hasRegistry = true
	}

	// 如果没有 registry，添加 docker.io/library/ 或 docker.io/
	if !hasRegistry {
		if slashIdx == -1 {
			// 只有镜像名，如 "nginx"
			image = "docker.io/library/" + image
		} else {
			// 有命名空间，如 "jgraph/drawio"
			image = "docker.io/" + image
		}
	}

	// 如果没有 tag，添加 :latest
	tagIdx := strings.LastIndex(image, ":")
	
	// 检查是否已经有 tag（: 在最后一个 / 之后）
	hasTag := tagIdx > strings.LastIndex(image, "/")

	if !hasTag {
		image = image + ":latest"
	}

	return image, nil
}

// GetImages 获取指定提供商的镜像源地址列表（向后兼容）
func (il *ImageList) GetImages(provider string) []string {
	entries, ok := il.Images[provider]
	if !ok {
		return []string{}
	}

	var images []string
	for _, entry := range entries {
		if entry.Valid {
			images = append(images, entry.Source)
		}
	}
	return images
}

// GetEntries 获取指定提供商的镜像条目列表（包含元数据）
func (il *ImageList) GetEntries(provider string) []ImageEntry {
	if entries, ok := il.Images[provider]; ok {
		return entries
	}
	return []ImageEntry{}
}

// Count 获取指定提供商的镜像数量
func (il *ImageList) Count(provider string) int {
	return len(il.GetImages(provider))
}

// CountValid 获取指定提供商的有效镜像数量
func (il *ImageList) CountValid(provider string) int {
	entries, ok := il.Images[provider]
	if !ok {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.Valid {
			count++
		}
	}
	return count
}

// GetInvalidEntries 获取无效的镜像条目（用于错误报告）
func (il *ImageList) GetInvalidEntries(provider string) []ImageEntry {
	entries, ok := il.Images[provider]
	if !ok {
		return []ImageEntry{}
	}
	var invalid []ImageEntry
	for _, entry := range entries {
		if !entry.Valid {
			invalid = append(invalid, entry)
		}
	}
	return invalid
}

// GetImagesWithDeduplication 获取镜像列表，并排除已在其他云商中存在的镜像
// 参数 currentProvider 是当前要处理的云商
// 返回的镜像列表不包含在其他云商中已存在的镜像
func (il *ImageList) GetImagesWithDeduplication(currentProvider string) []string {
	// 获取当前云商的所有有效镜像
	currentEntries := il.GetEntries(currentProvider)
	if len(currentEntries) == 0 {
		return []string{}
	}

	// 收集其他云商的所有镜像（标准化后的地址）
	otherProvidersImages := make(map[string]bool)
	for provider, entries := range il.Images {
		if provider == currentProvider {
			continue
		}
		for _, entry := range entries {
			if entry.Valid {
				otherProvidersImages[entry.Source] = true
			}
		}
	}

	// 过滤掉已在其他云商中存在的镜像，以及同一云商内重复的条目
	var uniqueImages []string
	seen := make(map[string]bool)
	for _, entry := range currentEntries {
		if !entry.Valid {
			continue
		}
		if otherProvidersImages[entry.Source] || seen[entry.Source] {
			// 镜像已在其他云商中存在，或本云商内已经排过
			continue
		}
		seen[entry.Source] = true
		uniqueImages = append(uniqueImages, entry.Source)
	}

	return uniqueImages
}

// GetDuplicateImages 获取在当前云商中与其他云商重复的镜像列表
// 用于报告哪些镜像被跳过了
func (il *ImageList) GetDuplicateImages(currentProvider string) map[string][]string {
	duplicates := make(map[string][]string)

	// 获取当前云商的所有有效镜像
	currentEntries := il.GetEntries(currentProvider)
	if len(currentEntries) == 0 {
		return duplicates
	}

	// 收集每个镜像在哪些云商中存在
	imageProviders := make(map[string][]string)
	for provider, entries := range il.Images {
		for _, entry := range entries {
			if entry.Valid {
				imageProviders[entry.Source] = append(imageProviders[entry.Source], provider)
			}
		}
	}

	// 找出在当前云商中与其他云商重复的镜像
	for _, entry := range currentEntries {
		if !entry.Valid {
			continue
		}
		providers := imageProviders[entry.Source]
		if len(providers) > 1 {
			// 镜像在多个云商中存在
			var otherProviders []string
			for _, p := range providers {
				if p != currentProvider {
					otherProviders = append(otherProviders, p)
				}
			}
			if len(otherProviders) > 0 {
				duplicates[entry.Source] = otherProviders
			}
		}
	}

	return duplicates
}
