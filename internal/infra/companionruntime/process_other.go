//go:build !darwin && !linux

package companionruntime

import "os/exec"

func configureDetachedProcess(*exec.Cmd) {}
