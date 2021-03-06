package download

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/gomods/athens/pkg/config"
	"github.com/gomods/athens/pkg/errors"
	"github.com/gomods/athens/pkg/module"
	"github.com/gomods/athens/pkg/observ"
	"github.com/gomods/athens/pkg/storage"
	"github.com/spf13/afero"
)

// UpstreamLister retrieves a list of available module versions from upstream
// i.e. VCS, and a Storage backend.
type UpstreamLister interface {
	List(ctx context.Context, mod string) (*storage.RevInfo, []string, error)
}

type listResp struct {
	Path     string
	Version  string
	Versions []string `json:",omitempty"`
	Time     time.Time
}

type vcsLister struct {
	goBinPath string
	fs        afero.Fs
}

func (l *vcsLister) List(ctx context.Context, mod string) (*storage.RevInfo, []string, error) {
	const op errors.Op = "vcsLister.List"
	ctx, span := observ.StartSpan(ctx, op.String())
	defer span.End()

	tmpDir, err := afero.TempDir(l.fs, "", "go-list")
	if err != nil {
		return nil, nil, errors.E(op, err)
	}
	defer l.fs.RemoveAll(tmpDir)

	cmd := exec.Command(
		l.goBinPath,
		"list", "-m", "-versions", "-json",
		config.FmtModVer(mod, "latest"),
	)
	cmd.Dir = tmpDir
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	gopath, err := afero.TempDir(l.fs, "", "athens")
	if err != nil {
		return nil, nil, errors.E(op, err)
	}
	defer module.ClearFiles(l.fs, gopath)
	cmd.Env = module.PrepareEnv(gopath)

	err = cmd.Run()
	if err != nil {
		err = fmt.Errorf("%v: %s", err, stderr)
		// as of now, we can't recognize between a true NotFound
		// and an unexpected error, so we choose the more
		// hopeful path of NotFound. This way the Go command
		// will not log en error and we still get to log
		// what happened here if someone wants to dig in more.
		// Once, https://github.com/golang/go/issues/30134 is
		// resolved, we can hopefully differentiate.
		return nil, nil, errors.E(op, err, errors.KindNotFound)
	}

	var lr listResp
	err = json.NewDecoder(stdout).Decode(&lr)
	if err != nil {
		return nil, nil, errors.E(op, err)
	}
	rev := storage.RevInfo{
		Time:    lr.Time,
		Version: lr.Version,
	}
	return &rev, lr.Versions, nil
}

// NewVCSLister creates an UpstreamLister which uses VCS to fetch a list of available versions
func NewVCSLister(goBinPath string, fs afero.Fs) UpstreamLister {
	return &vcsLister{goBinPath: goBinPath, fs: fs}
}
