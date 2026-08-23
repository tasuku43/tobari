package dockerruntime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const maxContextPolicyBytes = 64 * 1024

func decodeContextPolicy(data []byte) (tobari.ManifestPolicy, []byte, string, error) {
	if len(data) == 0 || len(data) > maxContextPolicyBytes {
		return tobari.ManifestPolicy{}, nil, "", fmt.Errorf("Context policy size is invalid")
	}
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return tobari.ManifestPolicy{}, nil, "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var policy tobari.ManifestPolicy
	if err := decoder.Decode(&policy); err != nil {
		return tobari.ManifestPolicy{}, nil, "", fmt.Errorf("decode Context policy: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return tobari.ManifestPolicy{}, nil, "", fmt.Errorf("Context policy contains trailing data")
	}
	return tobari.NormalizeContextPolicy(policy)
}

func (r *Runtime) contextPolicyPath(name string) string {
	return filepath.Join(r.contextPolicyDirectory(name), "context.json")
}

func defaultContextPolicyBytes() (tobari.ManifestPolicy, []byte, string, error) {
	policy, ok := tobari.DefaultContextPolicySnapshot()
	if !ok {
		return tobari.ManifestPolicy{}, nil, "", fmt.Errorf("default Context policy is unavailable")
	}
	return tobari.NormalizeContextPolicy(policy)
}

func (r *Runtime) ensureContextPolicy(manifest tobari.WorkspaceManifest, snapshot []byte) error {
	path := r.contextPolicyPath(manifest.Name)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maxContextPolicyBytes {
			return fmt.Errorf("Context policy snapshot is unsafe")
		}
		data, err := os.ReadFile(path) // #nosec G304 -- path is an owned Context child.
		if err != nil {
			return err
		}
		_, normalized, revision, err := decodeContextPolicy(data)
		if err != nil {
			return err
		}
		if revision != manifest.PolicyRevision || !bytes.Equal(data, normalized) {
			return fmt.Errorf("Context policy snapshot does not match its manifest")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if len(snapshot) == 0 {
		_, generated, _, err := defaultContextPolicyBytes()
		if err != nil {
			return err
		}
		snapshot = generated
	}
	_, normalized, revision, err := decodeContextPolicy(snapshot)
	if err != nil {
		return err
	}
	if revision != manifest.PolicyRevision || !bytes.Equal(snapshot, normalized) {
		return fmt.Errorf("Context policy snapshot changed before Context initialization")
	}
	return initializeBytes(path, normalized, 0o600)
}

func (r *Runtime) readContextPolicy(manifest tobari.WorkspaceManifest) (tobari.ManifestPolicy, error) {
	path := r.contextPolicyPath(manifest.Name)
	info, err := os.Lstat(path)
	if err != nil {
		return tobari.ManifestPolicy{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maxContextPolicyBytes {
		return tobari.ManifestPolicy{}, fmt.Errorf("Context policy snapshot is unsafe")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- owned Context child.
	if err != nil {
		return tobari.ManifestPolicy{}, err
	}
	policy, normalized, revision, err := decodeContextPolicy(data)
	if err != nil {
		return tobari.ManifestPolicy{}, err
	}
	if revision != manifest.PolicyRevision {
		return tobari.ManifestPolicy{}, fmt.Errorf("Context policy revision mismatch")
	}
	if !bytes.Equal(data, normalized) {
		return tobari.ManifestPolicy{}, fmt.Errorf("Context policy snapshot is not normalized")
	}
	return policy, nil
}
