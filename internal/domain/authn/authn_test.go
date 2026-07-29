package authn

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func validRequirement() Requirement {
	return Requirement{
		Methods:              []Method{MethodOAuth2, MethodPAT},
		Authority:            "example-authority",
		Audience:             "example-api",
		AccountID:            "account-1",
		RequiredCapabilities: []string{"items:read"},
	}
}

func validSession() Session {
	return Session{
		Method:              MethodOAuth2,
		Authority:           "example-authority",
		Audience:            "example-api",
		SubjectID:           "subject-1",
		AccountID:           "account-1",
		BindingID:           BindingID{value: "binding-1"},
		GrantedCapabilities: []string{"items:read", "items:write"},
		ExpiresAt:           time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestBindingIDIsOpaqueValidatedAndExcludedFromSessionJSON(t *testing.T) {
	const canary = "ephemeral-binding-secret-canary"
	binding, err := NewBindingID(canary)
	if err != nil {
		t.Fatalf("NewBindingID() error = %v", err)
	}
	if err := binding.Validate(); err != nil {
		t.Fatalf("BindingID.Validate() error = %v", err)
	}
	for _, invalid := range []string{"", " leading", "line\nbreak", "unsafe\u2028separator"} {
		if _, err := NewBindingID(invalid); err == nil {
			t.Errorf("NewBindingID(%q) succeeded", invalid)
		}
	}

	session := validSession()
	session.BindingID = binding
	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("binding")) {
		t.Fatalf("Session JSON exposed binding metadata: %s", encoded)
	}
	standalone, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{
		string(standalone),
		fmt.Sprint(binding),
		fmt.Sprintf("%+v", binding),
		fmt.Sprintf("%#v", binding),
		fmt.Sprint(session),
		fmt.Sprintf("%+v", session),
		fmt.Sprintf("%#v", session),
	} {
		if strings.Contains(rendered, canary) {
			t.Fatalf("generic formatting exposed BindingID: %s", rendered)
		}
	}
	bindingType := reflect.TypeOf(BindingID{})
	if bindingType.NumField() != 1 || bindingType.Field(0).IsExported() {
		t.Fatalf("BindingID representation is publicly extractable: %+v", bindingType)
	}
	sessionField, exists := reflect.TypeOf(Session{}).FieldByName("BindingID")
	if !exists || sessionField.Tag.Get("json") != "-" {
		t.Fatalf("Session.BindingID JSON tag = %q, want -", sessionField.Tag.Get("json"))
	}
}

func TestRequirementAndSessionValidate(t *testing.T) {
	if err := validRequirement().Validate(); err != nil {
		t.Fatalf("Requirement.Validate() error = %v", err)
	}
	if err := validSession().Validate(); err != nil {
		t.Fatalf("Session.Validate() error = %v", err)
	}

	pat := validSession()
	pat.Method = MethodPAT
	pat.ExpiresAt = time.Time{}
	if err := pat.Validate(); err != nil {
		t.Fatalf("PAT Session.Validate() error = %v", err)
	}
}

func TestZeroAndIncompleteAuthenticationMetadataFailClosed(t *testing.T) {
	requirements := []Requirement{
		{},
		{Methods: []Method{MethodUnknown}, Authority: "authority", Audience: "api"},
		{Methods: []Method{MethodPAT, MethodPAT}, Authority: "authority", Audience: "api"},
		{Methods: []Method{MethodPAT}, Audience: "api"},
		{Methods: []Method{MethodPAT}, Authority: "authority"},
		{Methods: []Method{MethodPAT}, Authority: "authority", Audience: "api", RequiredCapabilities: []string{"read", "read"}},
	}
	for index, requirement := range requirements {
		if err := requirement.Validate(); err == nil {
			t.Errorf("requirement case %d validated", index)
		}
	}

	missingBinding := validSession()
	missingBinding.BindingID = BindingID{}
	sessions := []Session{
		{},
		{Method: MethodPAT, Authority: "authority", Audience: "api"},
		{Method: MethodUnknown, Authority: "authority", Audience: "api", SubjectID: "subject"},
		{Method: MethodPAT, Authority: "authority", Audience: "api", SubjectID: "subject", GrantedCapabilities: []string{"read", "read"}},
		missingBinding,
	}
	for index, session := range sessions {
		if err := session.Validate(); err == nil {
			t.Errorf("session case %d validated", index)
		}
	}
}

func TestRequirementSatisfiesSessionExactly(t *testing.T) {
	now := time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := validRequirement().Satisfies(validSession(), now); err != nil {
		t.Fatalf("Satisfies() error = %v", err)
	}

	tests := []struct {
		name string
		kind MismatchKind
		edit func(*Requirement, *Session)
	}{
		{name: "method", kind: MismatchMethod, edit: func(requirement *Requirement, session *Session) {
			requirement.Methods = []Method{MethodOAuth2}
			session.Method = MethodPAT
			session.ExpiresAt = time.Time{}
		}},
		{name: "authority", kind: MismatchAuthority, edit: func(_ *Requirement, session *Session) { session.Authority = "other-authority" }},
		{name: "audience", kind: MismatchAudience, edit: func(_ *Requirement, session *Session) { session.Audience = "other-api" }},
		{name: "account", kind: MismatchAccount, edit: func(_ *Requirement, session *Session) { session.AccountID = "account-2" }},
		{name: "capability", kind: MismatchCapability, edit: func(_ *Requirement, session *Session) { session.GrantedCapabilities = []string{"Items:read"} }},
		{name: "expired", kind: MismatchExpired, edit: func(_ *Requirement, session *Session) { session.ExpiresAt = now }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requirement := validRequirement()
			session := validSession()
			test.edit(&requirement, &session)
			err := requirement.Satisfies(session, now)
			var mismatch *Mismatch
			if !errors.As(err, &mismatch) || mismatch.Kind != test.kind {
				t.Fatalf("Satisfies() error = %#v, want mismatch %q", err, test.kind)
			}
		})
	}
}

func TestAuthenticationMetadataErrorsDoNotEchoRejectedValues(t *testing.T) {
	const canary = "secret-canary-value"
	requirement := validRequirement()
	requirement.RequiredCapabilities = []string{canary + "\n"}
	err := requirement.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded")
	}
	if strings.Contains(err.Error(), canary) {
		t.Fatalf("validation error exposed rejected metadata: %v", err)
	}
	_, err = NewBindingID(canary + "\n")
	if err == nil || strings.Contains(err.Error(), canary) {
		t.Fatalf("binding validation exposed rejected metadata: %v", err)
	}
}

func TestAuthenticationMetadataHasNoCredentialBearingFields(t *testing.T) {
	for _, value := range []any{Requirement{}, Session{}} {
		typeOf := reflect.TypeOf(value)
		for index := 0; index < typeOf.NumField(); index++ {
			field := strings.ToLower(typeOf.Field(index).Name + " " + typeOf.Field(index).Tag.Get("json"))
			for _, forbidden := range []string{"token", "secret", "credential", "password", "authorization"} {
				if strings.Contains(field, forbidden) {
					t.Errorf("%s contains credential-bearing field %q", typeOf.Name(), typeOf.Field(index).Name)
				}
			}
		}
	}
}

func TestCloneOwnsIndependentSlices(t *testing.T) {
	requirement := validRequirement()
	requirementClone := requirement.Clone()
	requirementClone.Methods[0] = MethodPAT
	requirementClone.RequiredCapabilities[0] = "changed"
	if requirement.Methods[0] != MethodOAuth2 || requirement.RequiredCapabilities[0] != "items:read" {
		t.Fatal("Requirement.Clone() retained shared slice storage")
	}

	session := validSession()
	sessionClone := session.Clone()
	sessionClone.GrantedCapabilities[0] = "changed"
	if session.GrantedCapabilities[0] != "items:read" {
		t.Fatal("Session.Clone() retained shared slice storage")
	}
}

func TestUserConfigurationIsBoundedSecretFreeAndCloneable(t *testing.T) {
	configuration := UserConfiguration{
		SchemaVersion: UserConfigurationSchemaVersion,
		Method:        MethodOAuth2,
		Parameters: []PublicParameter{
			{Name: "public_client_id", Value: "example-public-client"},
			{Name: "redirect_uri", Value: "http://127.0.0.1/callback"},
		},
	}
	if err := configuration.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	clone := configuration.Clone()
	clone.Parameters[0].Value = "changed"
	if configuration.Parameters[0].Value != "example-public-client" {
		t.Fatal("UserConfiguration.Clone() retained shared storage")
	}

	invalid := []UserConfiguration{
		{},
		{SchemaVersion: 2, Method: MethodOAuth2, Parameters: []PublicParameter{}},
		{SchemaVersion: 1, Method: MethodUnknown, Parameters: []PublicParameter{}},
		{SchemaVersion: 1, Method: MethodPAT, Parameters: nil},
		{SchemaVersion: 1, Method: MethodPAT, Parameters: []PublicParameter{{Name: "Bad-Name", Value: "value"}}},
		{SchemaVersion: 1, Method: MethodPAT, Parameters: []PublicParameter{{Name: "refresh_token", Value: "forbidden"}}},
		{SchemaVersion: 1, Method: MethodPAT, Parameters: []PublicParameter{{Name: "name", Value: "value"}, {Name: "name", Value: "other"}}},
	}
	for index, candidate := range invalid {
		if err := candidate.Validate(); err == nil {
			t.Errorf("invalid configuration %d passed validation", index)
		}
	}

	typeOf := reflect.TypeOf(UserConfiguration{})
	for index := 0; index < typeOf.NumField(); index++ {
		field := strings.ToLower(typeOf.Field(index).Name + " " + typeOf.Field(index).Tag.Get("json"))
		for _, forbidden := range []string{"token", "secret", "credential", "password", "authorization", "verifier", "code"} {
			if strings.Contains(field, forbidden) {
				t.Errorf("UserConfiguration contains credential-bearing field %q", typeOf.Field(index).Name)
			}
		}
	}
}
