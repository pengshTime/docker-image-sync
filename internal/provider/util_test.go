package provider

import (
	"strings"
	"testing"
)

func TestParseImage(t *testing.T) {
	cases := []struct {
		in   string
		want ParsedImage
	}{
		{"docker.io/library/nginx:1.25", ParsedImage{Registry: "docker.io", Namespace: "library", Name: "nginx", Tag: "1.25"}},
		{"docker.io/jgraph/drawio:latest", ParsedImage{Registry: "docker.io", Namespace: "jgraph", Name: "drawio", Tag: "latest"}},
		{"registry.example.com/a/b/c:v1", ParsedImage{Registry: "registry.example.com", Namespace: "a_b", Name: "c", Tag: "v1"}},
		{"docker.io/library/nginx", ParsedImage{Registry: "docker.io", Namespace: "library", Name: "nginx", Tag: "latest"}},
		// 带端口的 registry 不能被误判成 tag
		{"localhost:5000/app/nginx", ParsedImage{Registry: "localhost:5000", Namespace: "app", Name: "nginx", Tag: "latest"}},
	}

	for _, c := range cases {
		got := ParseImage(c.in)
		if got != c.want {
			t.Errorf("ParseImage(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestBuildTargetImage(t *testing.T) {
	img := ParseImage("docker.io/somebody/node-exporter:v1.8.2")
	if got := BuildTargetImage("registry.cn-hangzhou.aliyuncs.com", "myns", img, false); got != "registry.cn-hangzhou.aliyuncs.com/myns/node_exporter:v1.8.2" {
		t.Errorf("no prefix = %q", got)
	}
	if got := BuildTargetImage("registry.cn-hangzhou.aliyuncs.com", "myns", img, true); got != "registry.cn-hangzhou.aliyuncs.com/myns/somebody_node_exporter:v1.8.2" {
		t.Errorf("with prefix = %q", got)
	}

	// latest 走默认 tag，地址里不追加 :latest
	latest := ParseImage("docker.io/jgraph/drawio:latest")
	if got := BuildTargetImage("reg.example.com", "ns", latest, false); got != "reg.example.com/ns/drawio" {
		t.Errorf("latest = %q", got)
	}

	// 阿里云用 usePrefix=false，源命名空间被丢掉，同名镜像会落到同一个仓库名上
	a := ParseImage("docker.io/aaa/toolbox:latest")
	b := ParseImage("docker.io/bbb/toolbox:latest")
	if BuildTargetImage("reg", "ns", a, false) != BuildTargetImage("reg", "ns", b, false) {
		t.Error("expected same-name images to collide without prefix")
	}
	if BuildTargetImage("reg", "ns", a, true) == BuildTargetImage("reg", "ns", b, true) {
		t.Error("prefix mode should keep namespaces apart")
	}
}

func TestSkopeoCopyArgs(t *testing.T) {
	t.Setenv("PREFERRED_ARCH", "")
	args := strings.Join(skopeoCopyArgs("docker.io/library/nginx:latest", "reg/ns/nginx:latest"), " ")
	if !strings.Contains(args, "--all --format docker") {
		t.Errorf("default args = %q, want multi-arch docker media type", args)
	}
	if strings.Contains(args, "--override-arch") {
		t.Errorf("default args must not pin an arch: %q", args)
	}

	t.Setenv("PREFERRED_ARCH", "amd64")
	args = strings.Join(skopeoCopyArgs("docker.io/library/nginx:latest", "reg/ns/nginx:latest"), " ")
	if !strings.Contains(args, "--override-arch amd64 --override-os linux") {
		t.Errorf("pinned args = %q", args)
	}
	if strings.Contains(args, "--all") {
		t.Errorf("pinned args must not use --all: %q", args)
	}
	if !strings.HasSuffix(args, "docker://docker.io/library/nginx:latest docker://reg/ns/nginx:latest") {
		t.Errorf("args endpoints = %q", args)
	}
}

func TestTailTruncatesLongOutput(t *testing.T) {
	long := strings.Repeat("a", 600)
	got := tail([]byte(long))
	if len(got) != 515 { // 3 个点的省略前缀 + 512 个字符
		t.Errorf("tail length = %d, want 515: %q", len(got), got)
	}
	if !strings.HasPrefix(got, "...") {
		t.Errorf("tail = %q, want it to start with the ellipsis marker", got)
	}
	if tail([]byte("short")) != "short" {
		t.Error("short output should pass through")
	}
}
