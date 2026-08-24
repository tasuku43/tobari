package dockerruntime

import (
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestServiceBrowserCommandAcceptsOnlyExactGeneratedRootURL(t *testing.T) {
	target, err := tobari.ServiceExposureURL(54321, "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	for _, goos := range []string{"darwin", "linux"} {
		executable, args, ok := serviceBrowserCommand(goos, target)
		if !ok || executable == "" || len(args) != 1 || args[0] != target {
			t.Fatalf("%s command = %q %#v %t", goos, executable, args, ok)
		}
	}
	for _, invalid := range []string{"http://127.0.0.1:54321/", target + "path", target + "?query"} {
		if _, _, ok := serviceBrowserCommand("darwin", invalid); ok {
			t.Errorf("invalid browser target passed: %s", invalid)
		}
	}
}
