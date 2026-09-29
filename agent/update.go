package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/update"
	"github.com/dhamith93/SyMon/internal/version"
)

// An admin can ask an agent to update itself on the dashboard. The
// collector passes the request on in its reply to a ping. The agent
// downloads the build the dashboard hands out, and installs it only when it
// is signed with the key built into this agent, runs on this machine, and
// is the version asked for.

const (
	updateTimeout = 5 * time.Minute
	// no agent build comes close to this
	maxBuildSize = 200 << 20
)

// updater tries each request once, so a failed update is not retried every
// minute, and keeps the error for the next ping to report
type updater struct {
	mu        sync.Mutex
	tried     int64
	lastError string
	// install is selfUpdate, replaced in tests. It returns only on failure.
	install func(*api.AgentUpdate) error
}

func newUpdater() *updater {
	return &updater{install: selfUpdate}
}

func (u *updater) handle(request *api.AgentUpdate) {
	if request == nil || request.Version == version.String() {
		return
	}
	u.mu.Lock()
	if request.RequestedAt == u.tried {
		u.mu.Unlock()
		return
	}
	u.tried = request.RequestedAt
	u.mu.Unlock()

	logger.Log("info", "updating to "+request.Version+" as asked on the dashboard")
	err := u.install(request)
	logger.Log("error", "cannot update to "+request.Version+": "+err.Error())
	u.mu.Lock()
	u.lastError = err.Error()
	u.mu.Unlock()
}

func (u *updater) err() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastError
}

// selfUpdate replaces this agent with the build asked for and restarts into
// it. It returns only when that failed.
func selfUpdate(request *api.AgentUpdate) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()
	build, signature, err := fetchBuild(ctx, request.DownloadUrl)
	if err != nil {
		return err
	}
	if err := installBuild(exe, build, signature, request.Version); err != nil {
		return err
	}
	logger.Log("info", "updated to "+request.Version+", restarting")
	return syscall.Exec(exe, os.Args, os.Environ())
}

// fetchBuild downloads this machine's build and its signature from the
// dashboard
func fetchBuild(ctx context.Context, dashboard string) ([]byte, []byte, error) {
	url := strings.TrimRight(dashboard, "/") + "/downloads/agent-linux-" + runtime.GOARCH
	build, err := download(ctx, url)
	if err != nil {
		return nil, nil, err
	}
	signature, err := download(ctx, url+".sig")
	if err != nil {
		return nil, nil, err
	}
	return build, signature, nil
}

func download(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", url, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBuildSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBuildSize {
		return nil, fmt.Errorf("%s is too big for an agent build", url)
	}
	return data, nil
}

// installBuild checks a build and puts it in place of exe
func installBuild(exe string, build []byte, signature []byte, want string) error {
	if err := update.Verify(build, signature); err != nil {
		return err
	}
	next := exe + ".new"
	if err := os.WriteFile(next, build, 0755); err != nil {
		return err
	}
	if err := os.Chmod(next, 0755); err != nil {
		os.Remove(next)
		return err
	}
	// the build has to run here and be the version asked for
	out, err := exec.Command(next, "-version").Output()
	if err != nil {
		os.Remove(next)
		return fmt.Errorf("the new build does not run here: %w", err)
	}
	if got := strings.TrimSpace(string(out)); got != "SyMon agent "+want {
		os.Remove(next)
		return fmt.Errorf("the new build says %q, not %s", got, want)
	}
	if err := os.Rename(next, exe); err != nil {
		os.Remove(next)
		return err
	}
	return nil
}
