package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pengshtime/docker-image-sync/internal/config"
	"github.com/pengshtime/docker-image-sync/internal/image"
	"github.com/pengshtime/docker-image-sync/internal/logger"
	"github.com/pengshtime/docker-image-sync/internal/notifier"
	"github.com/pengshtime/docker-image-sync/internal/provider"
)

const (
	Version = "1.0.0"
)

// BuildTime 由编译时注入: go build -ldflags "-X main.BuildTime=$(date +%Y-%m-%d)"
var BuildTime = "unknown"

func main() {
	// 命令行参数
	showVersion := flag.Bool("version", false, "Show version")
	showHelp := flag.Bool("help", false, "Show help")
	flag.Parse()

	if *showVersion {
		fmt.Printf("docker-image-sync version %s (built %s)\n", Version, BuildTime)
		os.Exit(0)
	}

	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	cfg := config.Load()

	// 初始化日志
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "INFO"
	}
	logger.Init(logLevel)

	logger.Info("Starting docker-image-sync v%s", Version)

	images, err := image.LoadFromFile(cfg.ImageList)
	if err != nil {
		logger.Fatal("Failed to load image list: %v", err)
	}

	// 获取镜像条目（包含元数据）
	entries := images.GetEntries(cfg.Provider)
	totalEntries := len(entries)

	if totalEntries == 0 {
		logger.Info("No images found for provider: %s", cfg.Provider)
		os.Exit(0)
	}

	// 检查并报告无效的镜像条目
	invalidEntries := images.GetInvalidEntries(cfg.Provider)
	if len(invalidEntries) > 0 {
		logger.Warn("Found %d invalid image entries:", len(invalidEntries))
		for _, entry := range invalidEntries {
			logger.Warn("  - %s: %s", entry.Raw, entry.ErrorMsg)
		}
	}

	// 检查跨云商重复的镜像
	duplicates := images.GetDuplicateImages(cfg.Provider)
	if len(duplicates) > 0 {
		logger.Warn("Found %d images that also exist in other providers:", len(duplicates))
		for img, providers := range duplicates {
			logger.Warn("  - %s (also in: %v)", img, providers)
		}
		logger.Warn("These images will be skipped to avoid duplicate pulls")
		logger.Info("")
	}

	// 获取有效的镜像地址列表（去重后）
	providerImages := images.GetImagesWithDeduplication(cfg.Provider)
	totalImages := len(providerImages)

	// 获取原始数量用于报告
	originalCount := len(images.GetImages(cfg.Provider))
	skippedDueToDedup := originalCount - totalImages

	if totalImages == 0 {
		if skippedDueToDedup > 0 {
			logger.Info("All %d images for provider %s already exist in other providers, skipping sync", skippedDueToDedup, cfg.Provider)
		} else {
			logger.Info("No valid images to sync for provider: %s", cfg.Provider)
		}
		os.Exit(0)
	}

	logger.Info("Loaded %d unique images for provider: %s (skipped %d duplicates)", totalImages, cfg.Provider, skippedDueToDedup)

	factory := provider.NewProviderFactory()
	p, err := factory.Create(cfg.Provider, cfg.Registry, cfg.Namespace, cfg.Username, cfg.Password)
	if err != nil {
		logger.Fatal("Failed to create provider: %v", err)
	}

	logger.Info("Using provider: %s (%s)", p.Name(), p.RegistryDomain())

	// 全局登录一次，避免并发登录竞争
	logger.Info("Logging in to registry...")
	if err := p.Login(); err != nil {
		logger.Fatal("Failed to login: %v", err)
	}
	logger.Info("Login successful")

	// 获取超时配置（默认300秒=5分钟）
	timeoutSec := 300
	if t := os.Getenv("SYNC_TIMEOUT"); t != "" {
		if parsed, err := fmt.Sscanf(t, "%d", &timeoutSec); err != nil || parsed != 1 {
			logger.Warn("Invalid SYNC_TIMEOUT value: %s, using default 300s", t)
			timeoutSec = 300
		}
	}
	logger.Debug("Using sync timeout: %ds", timeoutSec)

	// 获取重试次数（默认3次）
	maxRetries := 3
	if r := os.Getenv("MAX_RETRIES"); r != "" {
		if parsed, err := fmt.Sscanf(r, "%d", &maxRetries); err != nil || parsed != 1 {
			logger.Warn("Invalid MAX_RETRIES value: %s, using default 3", r)
			maxRetries = 3
		}
	}
	logger.Debug("Using max retries: %d", maxRetries)

	ctx := context.Background()

	var wg sync.WaitGroup
	imageChan := make(chan string, totalImages)
	resultChan := make(chan *provider.SyncResult, totalImages)

	for _, img := range providerImages {
		imageChan <- img
	}
	close(imageChan)

	// 启动进度条显示
	progressChan := make(chan struct{}, totalImages)
	go showProgress(totalImages, progressChan)

	concurrency := 3
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for img := range imageChan {
				result := syncWithRetry(ctx, p, img, timeoutSec, maxRetries)
				resultChan <- result
				progressChan <- struct{}{}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultChan)
		close(progressChan)
	}()

	var successCount, failureCount, skippedCount int
	var syncResults []*provider.SyncResult

	for result := range resultChan {
		syncResults = append(syncResults, result)
		if result.Success {
			if result.ErrorMessage == "already exists" {
				skippedCount++
				logger.Info("[SKIP] %s -> %s (already exists)", result.SourceImage, result.TargetImage)
			} else {
				successCount++
				logger.Info("[SUCCESS] %s -> %s", result.SourceImage, result.TargetImage)
			}
		} else {
			failureCount++
			logger.Error("[FAIL] %s -> %s: %s", result.SourceImage, result.TargetImage, result.ErrorMessage)
		}
	}

	logger.Info("")
	logger.Info("========================================")
	logger.Info("Sync Summary")
	logger.Info("========================================")
	logger.Info("Provider: %s", cfg.Provider)
	logger.Info("Total Entries: %d", totalEntries)
	logger.Info("Valid Images: %d", originalCount)
	if skippedDueToDedup > 0 {
		logger.Info("Skipped (duplicates): %d", skippedDueToDedup)
	}
	if len(invalidEntries) > 0 {
		logger.Info("Invalid Entries: %d", len(invalidEntries))
	}
	logger.Info("Success: %d", successCount)
	logger.Info("Skipped (exists): %d", skippedCount)
	logger.Info("Failed: %d", failureCount)
	logger.Info("========================================")

	// 钉钉推送通知
	dingNotifier := notifier.NewDingTalkNotifier()
	if dingNotifier.IsEnabled() {
		// 构建推送结果列表
		var dingResults []notifier.SyncResult
		for _, result := range syncResults {
			dingResults = append(dingResults, notifier.SyncResult{
				Success:      result.Success,
				SourceImage:  result.SourceImage,
				TargetImage:  result.TargetImage,
				ErrorMessage: result.ErrorMessage,
			})
		}

		// 构建钉钉消息
		msg := dingNotifier.BuildMessage(cfg.Provider, dingResults)

		// 转换为 JSON
		jsonStr, err := dingNotifier.ToJSON(msg)
		if err != nil {
			logger.Error("Failed to build DingTalk message: %v", err)
		} else {
			// 输出到环境变量
			if err := dingNotifier.OutputForEnv(jsonStr); err != nil {
				logger.Error("Failed to output DingTalk message: %v", err)
			} else {
				logger.Info("DingTalk notification message prepared")
			}
		}
	}

	if failureCount > 0 || len(invalidEntries) > 0 {
		os.Exit(1)
	}
}

// syncWithRetry 带重试机制的同步
func syncWithRetry(ctx context.Context, p provider.Provider, img string, timeoutSec, maxRetries int) *provider.SyncResult {
	var result *provider.SyncResult
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// 创建带超时的上下文
		ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
		
		result, lastErr = p.SyncImage(ctxTimeout, img)
		cancel()

		if lastErr == nil && result.Success {
			// 同步成功
			if attempt > 1 {
				logger.Debug("Sync succeeded after %d attempts", attempt)
			}
			return result
		}

		// 检查是否需要重试
		if lastErr != nil && isRetryableError(lastErr) && attempt < maxRetries {
			logger.Debug("Attempt %d failed for %s: %v, retrying...", attempt, img, lastErr)
			time.Sleep(time.Duration(attempt) * time.Second) // 指数退避
			continue
		}

		// 不可重试的错误或已达到最大重试次数
		break
	}

	if lastErr != nil {
		return &provider.SyncResult{
			SourceImage:  img,
			Success:      false,
			ErrorMessage: lastErr.Error(),
		}
	}

	return result
}

// showProgress 显示进度条（使用 stderr 避免与日志混合）
func showProgress(total int, progressChan <-chan struct{}) {
	completed := 0
	lastCompleted := -1
	
	for range progressChan {
		completed++
		// 只在进度变化时更新
		if completed != lastCompleted {
			lastCompleted = completed
			printProgressBar(completed, total)
		}
	}
	// 完成后换行
	fmt.Fprintln(os.Stderr, "")
}

// printProgressBar 打印进度条到 stderr
func printProgressBar(current, total int) {
	width := 30
	percent := float64(current) / float64(total)
	filled := int(float64(width) * percent)

	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}

	// 输出到 stderr，使用 \r 回到行首
	fmt.Fprintf(os.Stderr, "\r[%s] %d/%d (%.1f%%)", bar, current, total, percent*100)
}

// isRetryableError 判断错误是否可重试
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// 网络错误、超时错误可以重试
	return containsAny(errStr, []string{
		"timeout",
		"deadline exceeded",
		"connection refused",
		"no such host",
		"temporary",
		"retry",
		"i/o timeout",
	})
}

func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func printHelp() {
	fmt.Println("docker-image-sync - Sync Docker images to cloud registries")
	fmt.Println("")
	fmt.Println("Usage: docker-image-sync [options]")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  -version    Show version information")
	fmt.Println("  -help       Show this help message")
	fmt.Println("")
	fmt.Println("Environment Variables:")
	fmt.Println("  PROVIDER              Cloud provider (aliyun/huawei)")
	fmt.Println("  LOG_LEVEL             Log level (DEBUG/INFO/WARN/ERROR), default: INFO")
	fmt.Println("  SYNC_TIMEOUT          Sync timeout in seconds, default: 300 (5 minutes)")
	fmt.Println("  MAX_RETRIES           Max retry attempts, default: 3")
	fmt.Println("  IMAGE_LIST_FILE       Path to image list file, default: images.txt")
	fmt.Println("")
	fmt.Println("Provider specific variables:")
	fmt.Println("  {PROVIDER}_REGISTRY          Registry URL")
	fmt.Println("  {PROVIDER}_NAMESPACE         Namespace")
	fmt.Println("  {PROVIDER}_REGISTRY_USER     Username")
	fmt.Println("  {PROVIDER}_REGISTRY_PASSWORD Password")
}
