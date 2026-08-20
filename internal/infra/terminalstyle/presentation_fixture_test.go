package terminalstyle

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStructuredPresentationFixturePreservesItsAnswerKey(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "structured_output_fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	answerBytes, err := os.ReadFile(filepath.Join("testdata", "structured_output_answer.json"))
	if err != nil {
		t.Fatal(err)
	}
	var answer struct {
		SchemaVersion           int      `json:"schema_version"`
		Candidate               string   `json:"candidate"`
		VisibleBytesPreserved   bool     `json:"visible_bytes_preserved"`
		AllowedTokens           []string `json:"allowed_tokens"`
		MachineOutputChanged    bool     `json:"machine_output_changed"`
		ExternalProcessingCount int      `json:"external_processing_count"`
	}
	if err := json.Unmarshal(answerBytes, &answer); err != nil {
		t.Fatal(err)
	}
	if answer.SchemaVersion != 1 || answer.Candidate != "interactive_workspace_stdout" ||
		!answer.VisibleBytesPreserved || answer.MachineOutputChanged || answer.ExternalProcessingCount != 0 {
		t.Fatalf("invalid presentation answer key: %+v", answer)
	}
	if len(answer.AllowedTokens) == 0 {
		t.Fatal("presentation answer key has no token vocabulary")
	}

	styled, ok := ColorizeStructured(fixture)
	if !ok {
		t.Fatal("fixture is not a colorizable structured document")
	}
	if !bytes.Equal(stripSGR(styled), fixture) {
		t.Fatal("fixture visible bytes changed by color projection")
	}
}
