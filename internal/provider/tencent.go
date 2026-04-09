package provider

import (
	"context"
	"fmt"
)

type TencentProvider struct {
	registry  string
	namespace string
	username  string
	password  string
}

func NewTencentProvider(registry, namespace, username, password string) *TencentProvider {
	if registry == "" {
		registry = "ccr.ccs.tencentyun.com"
	}
	return &TencentProvider{
		registry:  registry,
		namespace: namespace,
		username:  username,
		password:  password,
	}
}

func (p *TencentProvider) Name() string {
	return "Tencent TCR"
}

func (p *TencentProvider) RegistryDomain() string {
	return p.registry
}

func (p *TencentProvider) Login() error {
	return dockerLogin(p.registry, p.username, p.password)
}

func (p *TencentProvider) SyncImage(ctx context.Context, sourceImage string) (*SyncResult, error) {
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

func (p *TencentProvider) CheckImageExists(ctx context.Context, image string) (bool, error) {
	return checkImageExists(ctx, image)
}

func (p *TencentProvider) buildTargetImage(sourceImage string) string {
	img := ParseImage(sourceImage)
	return BuildTargetImage(p.registry, p.namespace, img, true)
}
