package cli

import "testing"

// assertPublicNextArgvRoutes exercises the same root-option, catalog-routing,
// and typed-input path as a printed public continuation without invoking its
// side effect.
func assertPublicNextArgvRoutes(t *testing.T, argv []string) CommandSpec {
	t.Helper()
	if len(argv) == 0 || argv[0] != ProgramName {
		t.Fatalf("next argv = %q, want %q as argv[0]", argv, ProgramName)
	}
	options, commandArgs, err := parseRootOptions(argv[1:])
	if err != nil {
		t.Fatalf("next argv root parse = %q: %v", argv, err)
	}
	if len(commandArgs) == 0 {
		commandArgs = []string{"tobari"}
	}
	catalog := DefaultCatalog()
	commandArgs = normalizeRootAlias(commandArgs)
	commandArgs = normalizeTrailingHelpAlias(catalog, commandArgs)
	commandArgs = normalizeBareNamespace(catalog, commandArgs)
	command, rest, found := catalog.Match(commandArgs)
	if !found {
		t.Fatalf("next argv does not route through the catalog: %q", argv)
	}
	rest = normalizeLifecycleContextInput(command, options.WorkspaceManifestName, rest)
	if _, err := parseCommandInputs(command, rest); err != nil {
		t.Fatalf("next argv typed input parse = %q: %v", argv, err)
	}
	return command
}
