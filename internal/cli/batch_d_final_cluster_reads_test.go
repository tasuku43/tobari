package cli

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestBatchDClusterReadLogsHandlerIsExact(t *testing.T) {
	assertBatchDFinalClusterReadHandler(t, "cluster logs", "runFinalClusterLogs")
}

func TestBatchDClusterReadDenialsHandlerIsExact(t *testing.T) {
	assertBatchDFinalClusterReadHandler(t, "cluster denials", "runFinalClusterDenials")
}

func assertBatchDFinalClusterReadHandler(t *testing.T, path, want string) {
	t.Helper()
	spec, found := DefaultCatalog().lookupRegistered(path)
	if !found {
		t.Fatalf("%s is not registered", path)
	}
	name := runtime.FuncForPC(reflect.ValueOf(spec.handler).Pointer()).Name()
	if !strings.HasSuffix(name, "."+want) {
		t.Fatalf("%s handler = %s, want %s", path, name, want)
	}
}
