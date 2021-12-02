package buildah

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/werf/werf/pkg/buildah/types"
	"github.com/werf/werf/pkg/werf"
)

const (
	DefaultShmSize              = "65536k"
	BuildahImage                = "ghcr.io/werf/buildah:v1.22.3-1"
	BuildahStorageContainerName = "werf-buildah-storage"
)

type CommonOpts struct {
	LogWriter io.Writer
}

type BuildFromDockerfileOpts struct {
	CommonOpts
	ContextTar io.Reader
	BuildArgs  map[string]string
}

type RunMount struct {
	Type        string
	TmpfsSize   string
	Source      string
	Destination string
}

type RunCommandOpts struct {
	CommonOpts
	Args   []string
	Mounts []specs.Mount
}

type RmiOpts struct {
	CommonOpts
	Force bool
}

type (
	FromCommandOpts CommonOpts
	PushOpts        CommonOpts
	PullOpts        CommonOpts
	TagOpts         CommonOpts
	MountOpts       CommonOpts
	UmountOpts      CommonOpts
)

type Buildah interface {
	Tag(ctx context.Context, ref, newRef string, opts TagOpts) error
	Push(ctx context.Context, ref string, opts PushOpts) error
	BuildFromDockerfile(ctx context.Context, dockerfile []byte, opts BuildFromDockerfileOpts) (string, error)
	RunCommand(ctx context.Context, container string, command []string, opts RunCommandOpts) error
	FromCommand(ctx context.Context, container string, image string, opts FromCommandOpts) (string, error)
	Pull(ctx context.Context, ref string, opts PullOpts) error
	Inspect(ctx context.Context, ref string) (*types.BuilderInfo, error)
	Rmi(ctx context.Context, ref string, opts RmiOpts) error
	Mount(ctx context.Context, container string, opts MountOpts) (string, error)
	Umount(ctx context.Context, container string, opts UmountOpts) error
}

type Mode string

const (
	ModeAuto           Mode = "auto"
	ModeNativeRootless Mode = "native-rootless"
	ModeDockerWithFuse Mode = "docker-with-fuse"
)

func ProcessStartupHook(mode Mode) (bool, error) {
	switch ResolveMode(mode) {
	case ModeNativeRootless:
		return NativeRootlessProcessStartupHook(), nil
	case ModeDockerWithFuse:
		return false, nil
	default:
		return false, fmt.Errorf("unsupported mode %q", mode)
	}
}

type CommonBuildahOpts struct {
	TmpDir   string
	Insecure bool
}

type NativeRootlessModeOpts struct{}

type DockerWithFuseModeOpts struct{}

type BuildahOpts struct {
	CommonBuildahOpts
	DockerWithFuseModeOpts
	NativeRootlessModeOpts
}

func NewBuildah(mode Mode, opts BuildahOpts) (b Buildah, err error) {
	if opts.CommonBuildahOpts.TmpDir == "" {
		opts.CommonBuildahOpts.TmpDir = filepath.Join(werf.GetHomeDir(), "buildah", "tmp")
	}

	switch ResolveMode(mode) {
	case ModeNativeRootless:
		switch runtime.GOOS {
		case "linux":
			b, err = NewNativeRootlessBuildah(opts.CommonBuildahOpts, opts.NativeRootlessModeOpts)
			if err != nil {
				return nil, fmt.Errorf("unable to create new Buildah instance with mode %q: %s", mode, err)
			}
		default:
			panic("ModeNativeRootless can't be used on this OS")
		}
	case ModeDockerWithFuse:
		b, err = NewDockerWithFuseBuildah(opts.CommonBuildahOpts, opts.DockerWithFuseModeOpts)
		if err != nil {
			return nil, fmt.Errorf("unable to create new Buildah instance with mode %q: %s", mode, err)
		}
	default:
		return nil, fmt.Errorf("unsupported mode %q", mode)
	}

	return b, nil
}

func ResolveMode(mode Mode) Mode {
	switch mode {
	case ModeAuto:
		switch runtime.GOOS {
		case "linux":
			return ModeNativeRootless
		default:
			return ModeDockerWithFuse
		}
	default:
		return mode
	}
}

func debug() bool {
	return os.Getenv("WERF_BUILDAH_DEBUG") == "1"
}
