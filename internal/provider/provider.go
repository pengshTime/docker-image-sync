package provider

import "context"

type SyncResult struct {
	SourceImage  string
	TargetImage  string
	Success      bool
	ErrorMessage string
}

type Provider interface {
	Name() string
	RegistryDomain() string
	Login() error
	SyncImage(ctx context.Context, sourceImage string) (*SyncResult, error)
	CheckImageExists(ctx context.Context, image string) (bool, error)
}
