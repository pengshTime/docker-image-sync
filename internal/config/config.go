package config

import (
	"fmt"
	"os"
)

type Config struct {
	Provider   string
	Registry   string
	Namespace  string
	Username   string
	Password   string
	ImageList  string
}

func Load() *Config {
	provider := getEnv("PROVIDER", "aliyun")

	prefix := "ALIYUN"

	return &Config{
		Provider:  provider,
		Registry:  getEnv(fmt.Sprintf("%s_REGISTRY", prefix), ""),
		Namespace: getEnv(fmt.Sprintf("%s_NAMESPACE", prefix), ""),
		Username:  getEnv(fmt.Sprintf("%s_REGISTRY_USER", prefix), ""),
		Password:  getEnv(fmt.Sprintf("%s_REGISTRY_PASSWORD", prefix), ""),
		ImageList: getEnv("IMAGE_LIST_FILE", "images.txt"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
