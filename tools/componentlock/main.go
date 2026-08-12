// Command componentlock creates, verifies, and projects a release component lock.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/tasuku43/tobari/internal/domain/componentlock"
)

type evidence struct {
	SchemaVersion int      `json:"schema_version"`
	Image         string   `json:"image"`
	Digest        string   `json:"digest"`
	Revision      string   `json:"revision"`
	Platforms     []string `json:"platforms"`
}

func main() {
	if len(os.Args) < 2 {
		fatal(errors.New("usage: componentlock <create|verify|field> ..."))
	}
	switch os.Args[1] {
	case "create":
		if len(os.Args) != 8 {
			fatal(errors.New("usage: componentlock create <revision> <gateway-api> <gateway-evidence> <auth-broker-api> <auth-broker-evidence> <output>"))
		}
		create(os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6], os.Args[7])
	case "verify":
		if len(os.Args) != 4 {
			fatal(errors.New("usage: componentlock verify <lock> <revision>"))
		}
		lock := loadLock(os.Args[2])
		if lock.SourceRevision != os.Args[3] {
			fatal(errors.New("component lock source revision does not match requested revision"))
		}
	case "field":
		if len(os.Args) != 5 {
			fatal(errors.New("usage: componentlock field <lock> <revision> <gateway-image|gateway-api|auth-broker-image|auth-broker-api>"))
		}
		lock := loadLock(os.Args[2])
		if lock.SourceRevision != os.Args[3] {
			fatal(errors.New("component lock source revision does not match requested revision"))
		}
		switch os.Args[4] {
		case "gateway-image":
			fmt.Print(lock.Gateway.Reference())
		case "gateway-api":
			fmt.Print(lock.Gateway.API)
		case "auth-broker-image":
			fmt.Print(lock.AuthBroker.Reference())
		case "auth-broker-api":
			fmt.Print(lock.AuthBroker.API)
		default:
			fatal(errors.New("unknown component lock field"))
		}
	default:
		fatal(errors.New("usage: componentlock <create|verify|field> ..."))
	}
}

func create(revision, gatewayAPIText, gatewayPath, authAPIText, authPath, output string) {
	if _, err := os.Lstat(output); err == nil { // #nosec G703 -- output is the exact caller-selected staging path and is created with O_EXCL below.
		fatal(fmt.Errorf("component lock already exists; refusing to overwrite it: %s", output))
	} else if !errors.Is(err, os.ErrNotExist) {
		fatal(err)
	}
	gatewayAPI, err := strconv.Atoi(gatewayAPIText)
	if err != nil {
		fatal(errors.New("Gateway API must be an integer"))
	}
	authAPI, err := strconv.Atoi(authAPIText)
	if err != nil {
		fatal(errors.New("Auth Broker API must be an integer"))
	}
	gateway := loadEvidence(gatewayPath)
	auth := loadEvidence(authPath)
	if gateway.Revision != revision || auth.Revision != revision {
		fatal(errors.New("component evidence revision does not match requested revision"))
	}
	lock := componentlock.Lock{SchemaVersion: 1, SourceRevision: revision,
		Gateway:    componentlock.Component{Image: gateway.Image, Digest: gateway.Digest, API: gatewayAPI, Platforms: gateway.Platforms},
		AuthBroker: componentlock.Component{Image: auth.Image, Digest: auth.Digest, API: authAPI, Platforms: auth.Platforms}}
	if err := lock.Validate(); err != nil {
		fatal(err)
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		fatal(err)
	}
	data = append(data, '\n')
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 G703 -- release tooling intentionally accepts one explicit staging path; O_EXCL refuses existing files and symlinks.
	if err != nil {
		fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		fatal(err)
	}
	if err := file.Close(); err != nil {
		fatal(err)
	}
}

func loadEvidence(path string) evidence {
	var value evidence
	loadStrict(path, &value)
	if value.SchemaVersion != 1 {
		fatal(errors.New("component evidence schema_version must be 1"))
	}
	return value
}
func loadLock(path string) componentlock.Lock {
	file := openRegular(path)
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		fatal(err)
	}
	lock, err := componentlock.Parse(data)
	if err != nil {
		fatal(err)
	}
	return lock
}
func loadStrict(path string, value any) {
	file := openRegular(path)
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		fatal(errors.New("JSON input contains trailing data"))
	}
}

func openRegular(path string) *os.File {
	info, err := os.Lstat(path) // #nosec G703 -- componentlock intentionally accepts one explicit local release-evidence path.
	if err != nil {
		fatal(err)
	}
	if !info.Mode().IsRegular() {
		fatal(errors.New("component input must be a regular file"))
	}
	file, err := os.Open(path) // #nosec G304 G703 -- the exact caller-selected input was verified as a regular non-symlink immediately above.
	if err != nil {
		fatal(err)
	}
	return file
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "componentlock:", err); os.Exit(1) }
