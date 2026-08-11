package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
)

type buildIdentityFixture struct {
	SchemaVersion int `json:"schema_version"`
	Cases         []struct {
		Name                  string `json:"name"`
		Version               string `json:"version"`
		Commit                string `json:"commit"`
		ResolverChannel       string `json:"resolver_channel"`
		DevelopmentSource     bool   `json:"development_source"`
		GatewayRequiredAPI    int    `json:"gateway_required_api"`
		GatewaySelectedAPI    int    `json:"gateway_selected_api"`
		AuthBrokerRequiredAPI int    `json:"auth_broker_required_api"`
		AuthBrokerSelectedAPI int    `json:"auth_broker_selected_api"`
	} `json:"cases"`
}

type buildIdentityAnswers struct {
	SchemaVersion int `json:"schema_version"`
	Answers       []struct {
		Name                    string `json:"name"`
		Compatible              bool   `json:"compatible"`
		DevelopmentBuildCommand string `json:"development_build_command"`
		DevelopmentBinary       string `json:"development_binary"`
	} `json:"answers"`
}

func TestBuildIdentityFrozenSemanticCorpus(t *testing.T) {
	t.Parallel()
	fixtureData, err := os.ReadFile("testdata/build_identity_fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	answerData, err := os.ReadFile("testdata/build_identity_answer.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture buildIdentityFixture
	var answers buildIdentityAnswers
	if err := json.Unmarshal(fixtureData, &fixture); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(answerData, &answers); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 1 || answers.SchemaVersion != 1 || len(fixture.Cases) != 4 || len(answers.Answers) != len(fixture.Cases) {
		t.Fatalf("fixture shape = schema %d/%d cases %d/%d", fixture.SchemaVersion, answers.SchemaVersion, len(fixture.Cases), len(answers.Answers))
	}
	answerByName := make(map[string]struct {
		Compatible, Development bool
		Build, Binary           string
	}, len(answers.Answers))
	for _, answer := range answers.Answers {
		answerByName[answer.Name] = struct {
			Compatible, Development bool
			Build, Binary           string
		}{answer.Compatible, answer.DevelopmentBuildCommand != "", answer.DevelopmentBuildCommand, answer.DevelopmentBinary}
	}
	for _, item := range fixture.Cases {
		item := item
		t.Run(item.Name, func(t *testing.T) {
			t.Parallel()
			identity := buildidentity.Identity{
				Version: item.Version, Commit: item.Commit,
				ResolverChannel: buildidentity.ResolverChannel(item.ResolverChannel), DevelopmentSource: item.DevelopmentSource,
				Gateway:    buildidentity.Component{RequiredAPI: item.GatewayRequiredAPI, SelectedAPI: item.GatewaySelectedAPI},
				AuthBroker: buildidentity.Component{RequiredAPI: item.AuthBrokerRequiredAPI, SelectedAPI: item.AuthBrokerSelectedAPI},
			}
			if err := identity.Validate(); err != nil {
				t.Fatal(err)
			}
			answer, ok := answerByName[item.Name]
			if !ok || identity.Compatible() != answer.Compatible {
				t.Fatalf("compatibility = %t, answer = %+v", identity.Compatible(), answer)
			}
			build, binary, development := identity.DevelopmentRecovery()
			if build != answer.Build || binary != answer.Binary || development != answer.Development {
				t.Fatalf("recovery = %q %q %t, answer = %+v", build, binary, development, answer)
			}
			textOutput, err := renderVersion(identity, successFormatText, false)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(textOutput), "task build:dev") != answer.Development ||
				strings.Contains(string(textOutput), "bin/tobari-dev") != answer.Development {
				t.Fatalf("text recovery leakage = %q", textOutput)
			}
			jsonOutput, err := renderVersion(identity, successFormatJSON, false)
			if err != nil {
				t.Fatal(err)
			}
			var document versionJSONDocument
			if err := json.Unmarshal(jsonOutput, &document); err != nil {
				t.Fatal(err)
			}
			if document.SchemaVersion != 1 || document.BuildIdentity.Compatible != answer.Compatible ||
				document.BuildIdentity.DevelopmentBuildCommand != answer.Build || document.BuildIdentity.DevelopmentBinary != answer.Binary {
				t.Fatalf("JSON document = %+v", document)
			}
		})
	}
}
