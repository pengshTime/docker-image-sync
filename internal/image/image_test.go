package image

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeImageRef(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"nginx", "docker.io/library/nginx:latest", false},
		{"jgraph/drawio", "docker.io/jgraph/drawio:latest", false},
		{"corentinth/it-tools:2024.1", "docker.io/corentinth/it-tools:2024.1", false},
		{"docker.io/library/nginx:1.25", "docker.io/library/nginx:1.25", false},
		{"ghcr.io/owner/repo", "ghcr.io/owner/repo:latest", false},
		{"localhost:5000/app/nginx", "localhost:5000/app/nginx:latest", false},
		{"registry-1.docker.io/library/nginx", "registry-1.docker.io/library/nginx:latest", false},
		{"nginx@sha256:0000000000000000", "", true},
		{"", "", true},
	}

	for _, c := range cases {
		got, err := normalizeImageRef(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("normalizeImageRef(%q) expected error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeImageRef(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("normalizeImageRef(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLoadFromFileGroupsAndFlags(t *testing.T) {
	content := "# comment\n\n[aliyun]\njgraph/drawio:latest\nlinuxserver/calibre-web\nbad@sha256:abc\n\n[other]\njgraph/drawio:latest\n"
	path := filepath.Join(t.TempDir(), "images.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	list, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(list.Images) != 2 {
		t.Fatalf("expected 2 providers, got %d: %v", len(list.Images), list.Images)
	}

	// 条目按标准化后的地址比对
	if got := list.GetImages("aliyun"); len(got) != 2 {
		t.Errorf("aliyun valid images = %v, want 2 entries", got)
	} else if got[0] != "docker.io/jgraph/drawio:latest" || got[1] != "docker.io/linuxserver/calibre-web:latest" {
		t.Errorf("aliyun images = %v", got)
	}

	invalid := list.GetInvalidEntries("aliyun")
	if len(invalid) != 1 {
		t.Fatalf("expected 1 invalid entry, got %v", invalid)
	}
	if invalid[0].ErrorMsg == "" {
		t.Error("invalid entry should carry an error message")
	}

	// 跨云商重复的镜像在当前云商被过滤掉
	if deduped := list.GetImagesWithDeduplication("aliyun"); len(deduped) != 1 {
		t.Errorf("deduplicated aliyun images = %v, want only drawio removed", deduped)
	}
	if dups := list.GetDuplicateImages("aliyun"); len(dups) != 1 {
		t.Errorf("expected drawio reported as duplicate, got %v", dups)
	}
}

func TestGetImagesWithDeduplicationRemovesSameSectionDuplicates(t *testing.T) {
	content := "[aliyun]\nnginx:1.25\nnginx:1.25\nredis:7\n"
	path := filepath.Join(t.TempDir(), "images.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	list, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if got := list.GetImages("aliyun"); len(got) != 3 {
		t.Fatalf("GetImages should keep raw entries, got %v", got)
	}
	deduped := list.GetImagesWithDeduplication("aliyun")
	if len(deduped) != 2 {
		t.Errorf("deduplicated images = %v, want nginx + redis", deduped)
	}
}

func TestParseImageEntryAlias(t *testing.T) {
	entry := parseImageEntry("mypython=python:3.11-slim")
	if !entry.Valid {
		t.Fatalf("entry should be valid: %+v", entry)
	}
	if entry.Alias != "mypython" {
		t.Errorf("alias = %q, want mypython", entry.Alias)
	}
	// 别名目前只解析不参与目标命名，标准化结果必须来自等号右边
	if entry.Source != "docker.io/library/python:3.11-slim" {
		t.Errorf("source = %q", entry.Source)
	}
}
