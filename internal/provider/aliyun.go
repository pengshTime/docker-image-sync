package provider

import (
	"context"
	"fmt"
)

type AliyunProvider struct {
	registry  string
	namespace string
	username  string
	password  string
}

func NewAliyunProvider(registry, namespace, username, password string) *AliyunProvider {
	if registry == "" {
		registry = "registry.cn-hangzhou.aliyuncs.com"
	}
	return &AliyunProvider{
		registry:  registry,
		namespace: namespace,
		username:  username,
		password:  password,
	}
}

func (p *AliyunProvider) Name() string {
	return "Aliyun ACR"
}

func (p *AliyunProvider) RegistryDomain() string {
	return p.registry
}

func (p *AliyunProvider) Login() error {
	return dockerLogin(p.registry, p.username, p.password)
}

func (p *AliyunProvider) SyncImage(ctx context.Context, sourceImage string) (*SyncResult, error) {
	targetImage := p.buildTargetImage(sourceImage)
	result := &SyncResult{
		SourceImage: sourceImage,
		TargetImage: targetImage,
		Success:     false,
	}

	exists, err := checkImageExists(ctx, targetImage)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("failed to check image: %v", err)
		return result, err
	}
	if exists {
		result.Success = true
		result.ErrorMessage = "already exists"
		return result, nil
	}

	if err := skopeoCopy(ctx, sourceImage, targetImage); err != nil {
		result.ErrorMessage = fmt.Sprintf("copy failed: %v", err)
		return result, err
	}

	result.Success = true
	return result, nil
}

func (p *AliyunProvider) CheckImageExists(ctx context.Context, image string) (bool, error) {
	return checkImageExists(ctx, image)
}

func (p *AliyunProvider) buildTargetImage(sourceImage string) string {
	img := ParseImage(sourceImage)
	return BuildTargetImage(p.registry, p.namespace, img, false)
}
