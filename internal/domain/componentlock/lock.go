// Package componentlock defines the generated binding between one CLI release
// source revision and its published Gateway index.
package componentlock

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
)

const SchemaVersion = 1

var (
	revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type Component struct {
	Image     string   `json:"image"`
	Digest    string   `json:"digest"`
	API       int      `json:"api"`
	Platforms []string `json:"platforms"`
}

func (c Component) Reference() string { return c.Image + "@" + c.Digest }

type Lock struct {
	SchemaVersion  int       `json:"schema_version"`
	SourceRevision string    `json:"source_revision"`
	Gateway        Component `json:"gateway"`
}

func Parse(data []byte) (Lock, error) {
	var lock Lock
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&lock); err != nil {
		return Lock{}, fmt.Errorf("decode component lock: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Lock{}, fmt.Errorf("component lock contains trailing data")
		}
		return Lock{}, fmt.Errorf("decode component lock trailing data: %w", err)
	}
	if err := lock.Validate(); err != nil {
		return Lock{}, err
	}
	canonical, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return Lock{}, fmt.Errorf("encode canonical component lock: %w", err)
	}
	canonical = append(canonical, '\n')
	if !bytes.Equal(data, canonical) {
		return Lock{}, fmt.Errorf("component lock must use the canonical generated encoding")
	}
	return lock, nil
}

func (l Lock) Validate() error {
	if l.SchemaVersion != SchemaVersion {
		return fmt.Errorf("component lock schema_version must be %d", SchemaVersion)
	}
	if !revisionPattern.MatchString(l.SourceRevision) {
		return fmt.Errorf("component lock source_revision must be a full lowercase Git SHA")
	}
	if err := validateComponent("Gateway", l.Gateway.Image, l.Gateway.Digest, l.Gateway.Platforms, "ghcr.io/tasuku43/tobari/gateway"); err != nil {
		return err
	}
	if l.Gateway.API <= 0 {
		return fmt.Errorf("Gateway API must be positive")
	}
	return nil
}

func validateComponent(name, image, digest string, platforms []string, repository string) error {
	if image != repository {
		return fmt.Errorf("%s image must be %s", name, repository)
	}
	if !digestPattern.MatchString(digest) || strings.Trim(digest[7:], "0") == "" {
		return fmt.Errorf("%s digest must be a non-zero sha256 digest", name)
	}
	if !slices.Equal(platforms, []string{"linux/amd64", "linux/arm64"}) {
		return fmt.Errorf("%s platforms must be exactly linux/amd64 and linux/arm64", name)
	}
	return nil
}
