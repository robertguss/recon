package index

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
)

func CurrentGitState(ctx context.Context, moduleRoot string) (commit string, dirty bool) {
	rev := exec.CommandContext(ctx, "git", "-C", moduleRoot, "rev-parse", "HEAD")
	revOut, revErr := rev.Output()
	if revErr == nil {
		commit = strings.TrimSpace(string(revOut))
	}

	status := exec.CommandContext(ctx, "git", "-C", moduleRoot, "status", "--porcelain")
	statusOut, statusErr := status.Output()
	if statusErr == nil {
		dirty = len(bytes.TrimSpace(statusOut)) > 0
	}

	return commit, dirty
}
