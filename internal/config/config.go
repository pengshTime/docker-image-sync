package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Provider  string
	Registry  string
	Namespace string
	Username  string
	Password  string
	ImageList string
}

func Load() *Config {
	provider := getEnv("PROVIDER", "aliyun")
	prefix := strings.ToUpper(provider)

	return &Config{
		Provider:  provider,
		Registry:  getEnv(fmt.Sprintf("%s_REGISTRY", prefix), ""),
		Namespace: getEnv(fmt.Sprintf("%s_NAMESPACE", prefix), ""),
		Username:  getEnv(fmt.Sprintf("%s_REGISTRY_USER", prefix), ""),
		Password:  getEnv(fmt.Sprintf("%s_REGISTRY_PASSWORD", prefix), ""),
		ImageList: getEnv("IMAGE_LIST_FILE", "images.txt"),
	}
}

// MissingCredentials 返回缺失的必需配置项，便于给出明确错误而不是底层命令失败
func (c *Config) MissingCredentials() []string {
	prefix := strings.ToUpper(c.Provider)
	var missing []string
	for _, item := range []struct {
		env   string
		value string
	}{
		{fmt.Sprintf("%s_NAMESPACE", prefix), c.Namespace},
		{fmt.Sprintf("%s_REGISTRY_USER", prefix), c.Username},
		{fmt.Sprintf("%s_REGISTRY_PASSWORD", prefix), c.Password},
	} {
		if item.value == "" {
			missing = append(missing, item.env)
		}
	}
	return missing
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
