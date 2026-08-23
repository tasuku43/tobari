package cli

import "strings"

// expectedSurfaceText keeps presentation assertions honest across the
// release and research executables. Catalog paths remain the common
// WorkspaceEntryCommandPath; only the executable prefix is surface-specific.
func expectedSurfaceText(text string) string {
	if ProgramName == ReleaseProgramName {
		return text
	}
	text = strings.ReplaceAll(text, "`tobari`", "`"+ProgramName+"`")
	text = strings.ReplaceAll(text, "tobari ", ProgramName+" ")
	text = strings.ReplaceAll(text, "tobari\n", ProgramName+"\n")
	if strings.HasSuffix(text, "tobari") {
		text = strings.TrimSuffix(text, "tobari") + ProgramName
	}
	return text
}

func expectedSurfaceArgv(argv []string) []string {
	if len(argv) == 0 || ProgramName == ReleaseProgramName {
		return argv
	}
	copyArgv := append([]string(nil), argv...)
	if copyArgv[0] == ReleaseProgramName {
		copyArgv[0] = ProgramName
	}
	return copyArgv
}
