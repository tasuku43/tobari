// Package releaseversion validates release tags against SemVer 2.0 and the
// narrower repository release policy.
package releaseversion

import (
	"fmt"
	"regexp"
	"strings"
)

var semverPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

// Version is a validated SemVer tag with the repository-required leading v.
type Version struct {
	Tag        string
	Value      string
	Prerelease string
	Build      string
}

// Parse accepts the complete SemVer 2.0 grammar with a required leading v.
func Parse(tag string) (Version, error) {
	match := semverPattern.FindStringSubmatch(tag)
	if match == nil {
		return Version{}, fmt.Errorf("tag %q is not SemVer with a leading v", tag)
	}
	prerelease := match[4]
	for _, identifier := range strings.Split(prerelease, ".") {
		if prerelease == "" {
			break
		}
		if isNumeric(identifier) && len(identifier) > 1 && identifier[0] == '0' {
			return Version{}, fmt.Errorf("tag %q has a numeric prerelease identifier with a leading zero", tag)
		}
	}
	return Version{
		Tag:        tag,
		Value:      strings.TrimPrefix(tag, "v"),
		Prerelease: prerelease,
		Build:      match[5],
	}, nil
}

// ParseReleaseTag applies this repository's immutable-release policy. Build
// metadata is valid SemVer, but is intentionally excluded because two tags
// with equal SemVer precedence must not name different public artifacts.
func ParseReleaseTag(tag string) (Version, error) {
	version, err := Parse(tag)
	if err != nil {
		return Version{}, err
	}
	if version.Build != "" {
		return Version{}, fmt.Errorf("release tag %q uses build metadata, which repository release policy excludes", tag)
	}
	return version, nil
}

func (v Version) Stable() bool {
	return v.Prerelease == ""
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
