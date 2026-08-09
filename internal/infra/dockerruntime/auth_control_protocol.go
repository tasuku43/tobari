package dockerruntime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/companionruntime"
)

type brokerControlOperation string

const (
	// The broker-control wire protocol is versioned independently from the
	// credential-provider manifest, even though both initial schemas are v1.
	brokerControlSchemaVersion = 1

	brokerControlHealth           brokerControlOperation = "health"
	brokerControlUnlock           brokerControlOperation = "unlock"
	brokerControlStatus           brokerControlOperation = "status"
	brokerControlLogin            brokerControlOperation = "login"
	brokerControlImport           brokerControlOperation = "import"
	brokerControlLogout           brokerControlOperation = "logout"
	brokerControlIssueHandle      brokerControlOperation = "issue_handle"
	brokerControlBindingStatus    brokerControlOperation = "binding_status"
	brokerControlCompanionPrepare brokerControlOperation = "companion_prepare"
	brokerControlCompanionStatus  brokerControlOperation = "companion_status"
)

type brokerControlExpectation struct {
	Operation      brokerControlOperation
	Provider       string
	Revision       string
	EpochID        string
	ContextID      string
	AccountLabel   string
	DriverID       string
	DriverRevision string
}

// brokerMutationOutcomeUnknown marks a control call whose final broker frame
// was unavailable after a Context credential mutation may have committed. It
// is deliberately distinct from an authoritative broker error frame.
type brokerMutationOutcomeUnknown struct{}

func (brokerMutationOutcomeUnknown) Error() string {
	return "Auth Broker mutation acknowledgement is unavailable"
}

func brokerControlExpectationFor(arguments []string) (brokerControlExpectation, error) {
	if len(arguments) == 0 {
		return brokerControlExpectation{}, fmt.Errorf("Auth Broker control operation is missing")
	}
	expectation := brokerControlExpectation{Operation: brokerControlOperation(arguments[0])}
	switch expectation.Operation {
	case brokerControlHealth, brokerControlUnlock:
		return expectation, nil
	case brokerControlCompanionPrepare:
		if len(arguments) != 3 || arguments[1] != "--epoch-id" ||
			!companionruntime.ValidEpochID(arguments[2]) {
			return brokerControlExpectation{}, fmt.Errorf("Auth Broker companion epoch is invalid")
		}
		expectation.EpochID = arguments[2]
		return expectation, nil
	case brokerControlCompanionStatus:
		if len(arguments) != 1 {
			return brokerControlExpectation{}, fmt.Errorf("Auth Broker companion status arguments are invalid")
		}
		return expectation, nil
	case brokerControlLogin:
		return brokerLoginControlExpectation(arguments)
	case brokerControlStatus, brokerControlImport,
		brokerControlLogout, brokerControlIssueHandle, brokerControlBindingStatus:
		provider, err := brokerControlArgument(arguments, "--provider")
		if err != nil {
			return brokerControlExpectation{}, err
		}
		if err := authbroker.ValidateProviderID(provider); err != nil {
			return brokerControlExpectation{}, fmt.Errorf("Auth Broker control provider is invalid: %w", err)
		}
		expectation.Provider = provider
	default:
		return brokerControlExpectation{}, fmt.Errorf("Auth Broker control operation %q is unsupported", arguments[0])
	}
	if expectation.Operation == brokerControlBindingStatus {
		revision, err := brokerControlArgument(arguments, "--revision")
		if err != nil {
			return brokerControlExpectation{}, err
		}
		if !validAuthRevision(revision) {
			return brokerControlExpectation{}, fmt.Errorf("Auth Broker control revision is invalid")
		}
		expectation.Revision = revision
	}
	return expectation, nil
}

func brokerLoginControlExpectation(arguments []string) (brokerControlExpectation, error) {
	if len(arguments) != 7 && len(arguments) != 11 {
		return brokerControlExpectation{}, fmt.Errorf("Auth Broker login arguments are invalid")
	}
	if arguments[0] != string(brokerControlLogin) || arguments[1] != "--context-id" ||
		arguments[3] != "--provider" || arguments[5] != "--account-label" ||
		tobari.ValidateContextID(arguments[2]) != nil {
		return brokerControlExpectation{}, fmt.Errorf("Auth Broker login arguments are invalid")
	}
	expectation := brokerControlExpectation{
		Operation:    brokerControlLogin,
		ContextID:    arguments[2],
		Provider:     arguments[4],
		AccountLabel: arguments[6],
	}
	switch expectation.Provider {
	case "github":
		if len(arguments) != 7 || expectation.AccountLabel == "" ||
			authbroker.ValidateSecretFreeText("account label", expectation.AccountLabel, 128) != nil {
			return brokerControlExpectation{}, fmt.Errorf("Auth Broker GitHub login arguments are invalid")
		}
	case "aws":
		if len(arguments) != 11 || arguments[7] != "--driver-id" ||
			arguments[9] != "--driver-revision" ||
			!hostAWSAccountPattern.MatchString(expectation.AccountLabel) ||
			arguments[8] != awsHostDriverID ||
			!hostDriverRevisionPattern.MatchString(arguments[10]) {
			return brokerControlExpectation{}, fmt.Errorf("Auth Broker AWS login arguments are invalid")
		}
		expectation.DriverID = arguments[8]
		expectation.DriverRevision = arguments[10]
	default:
		return brokerControlExpectation{}, fmt.Errorf("Auth Broker login provider is invalid")
	}
	return expectation, nil
}

func brokerControlArgument(arguments []string, name string) (string, error) {
	value := ""
	found := false
	for index := 1; index < len(arguments); index++ {
		if arguments[index] != name {
			continue
		}
		if found || index+1 >= len(arguments) || arguments[index+1] == "" || strings.HasPrefix(arguments[index+1], "--") {
			return "", fmt.Errorf("Auth Broker control argument %s is missing or duplicated", name)
		}
		found = true
		value = arguments[index+1]
	}
	if !found {
		return "", fmt.Errorf("Auth Broker control argument %s is missing", name)
	}
	return value, nil
}

func (e brokerControlExpectation) mutationOutcomeSensitive() bool {
	switch e.Operation {
	case brokerControlLogin, brokerControlImport, brokerControlLogout:
		return true
	default:
		return false
	}
}

func decodeBrokerControlResponse(
	data []byte, expectation brokerControlExpectation,
) (brokerControlResponse, error) {
	fields, err := decodeBrokerJSONObject(data)
	if err != nil {
		return brokerControlResponse{}, err
	}
	schemaVersion, err := brokerRequiredField[int](fields, "schema_version")
	if err != nil || schemaVersion != brokerControlSchemaVersion {
		return brokerControlResponse{}, fmt.Errorf("Auth Broker response schema version is invalid")
	}
	ok, err := brokerRequiredField[bool](fields, "ok")
	if err != nil {
		return brokerControlResponse{}, fmt.Errorf("Auth Broker response success state is invalid")
	}
	response := brokerControlResponse{SchemaVersion: schemaVersion, OK: ok}
	if !ok {
		if err := requireBrokerFields(fields, "schema_version", "ok", "error"); err != nil {
			return brokerControlResponse{}, err
		}
		errorFields, err := decodeBrokerJSONObject(fields["error"])
		if err != nil {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker error response is invalid")
		}
		if err := requireBrokerFields(errorFields, "code"); err != nil {
			return brokerControlResponse{}, err
		}
		code, err := brokerRequiredField[string](errorFields, "code")
		if err != nil || !validBrokerControlCode(code) {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker error code is invalid")
		}
		response.Error = &struct {
			Code string `json:"code"`
		}{Code: code}
		return response, nil
	}

	switch expectation.Operation {
	case brokerControlHealth:
		if err := requireBrokerFields(fields, "schema_version", "ok", "state"); err != nil {
			return brokerControlResponse{}, err
		}
		response.State, err = brokerRequiredField[string](fields, "state")
		if err != nil || (response.State != "locked" && response.State != "unlocked") {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker health state is invalid")
		}
	case brokerControlUnlock:
		if err := requireBrokerFields(fields, "schema_version", "ok", "state"); err != nil {
			return brokerControlResponse{}, err
		}
		response.State, err = brokerRequiredField[string](fields, "state")
		if err != nil || response.State != "unlocked" {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker unlock state is invalid")
		}
	case brokerControlCompanionPrepare:
		if err := requireBrokerFields(fields, "schema_version", "ok", "state", "epoch_id"); err != nil {
			return brokerControlResponse{}, err
		}
		response.State, err = brokerRequiredField[string](fields, "state")
		if err != nil || response.State != "prepared" {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker companion prepare state is invalid")
		}
		response.EpochID, err = brokerRequiredField[string](fields, "epoch_id")
		if err != nil || response.EpochID != expectation.EpochID {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker companion prepare epoch does not match the request")
		}
	case brokerControlCompanionStatus:
		if err := requireBrokerFields(fields, "schema_version", "ok", "state", "epoch_id"); err != nil {
			return brokerControlResponse{}, err
		}
		response.State, err = brokerRequiredField[string](fields, "state")
		if err != nil || (response.State != "absent" && response.State != "prepared" && response.State != "ready") {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker companion status state is invalid")
		}
		response.EpochID, err = brokerRequiredField[string](fields, "epoch_id")
		if err != nil || (response.State == "absent" && response.EpochID != "") ||
			(response.State != "absent" && !companionruntime.ValidEpochID(response.EpochID)) {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker companion status epoch is invalid")
		}
	case brokerControlStatus:
		if err := decodeBrokerProviderState(fields, expectation, &response); err != nil {
			return brokerControlResponse{}, err
		}
		switch response.State {
		case "locked", "not_configured":
			if err := requireBrokerFields(fields, "schema_version", "ok", "state", "provider"); err != nil {
				return brokerControlResponse{}, err
			}
		case "ready":
			expected := []string{"schema_version", "ok", "state", "provider", "revision"}
			if _, present := fields["account_label"]; present {
				expected = append(expected, "account_label")
			}
			if err := requireBrokerFields(fields, expected...); err != nil {
				return brokerControlResponse{}, err
			}
			if err := decodeBrokerCredentialMetadata(fields, false, &response); err != nil {
				return brokerControlResponse{}, err
			}
		default:
			return brokerControlResponse{}, fmt.Errorf("Auth Broker provider status is invalid")
		}
	case brokerControlLogin, brokerControlImport:
		expected := []string{"schema_version", "ok", "provider", "revision"}
		requireAccountLabel := expectation.Operation == brokerControlLogin
		if requireAccountLabel {
			expected = append(expected, "account_label")
		}
		if err := requireBrokerFields(fields, expected...); err != nil {
			return brokerControlResponse{}, err
		}
		if err := decodeBrokerExpectedProvider(fields, expectation.Provider, &response); err != nil {
			return brokerControlResponse{}, err
		}
		if err := decodeBrokerCredentialMetadata(fields, requireAccountLabel, &response); err != nil {
			return brokerControlResponse{}, err
		}
		if expectation.Operation == brokerControlLogin &&
			(response.AccountLabel == nil || *response.AccountLabel != expectation.AccountLabel) {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker login account label does not match the request")
		}
	case brokerControlLogout:
		if err := requireBrokerFields(fields, "schema_version", "ok", "provider", "state", "changed"); err != nil {
			return brokerControlResponse{}, err
		}
		if err := decodeBrokerExpectedProvider(fields, expectation.Provider, &response); err != nil {
			return brokerControlResponse{}, err
		}
		response.State, err = brokerRequiredField[string](fields, "state")
		if err != nil || response.State != "logged_out" {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker logout state is invalid")
		}
		changed, changedErr := brokerRequiredField[bool](fields, "changed")
		if changedErr != nil {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker logout change state is invalid")
		}
		response.Changed = &changed
	case brokerControlIssueHandle:
		if err := requireBrokerFields(fields, "schema_version", "ok", "provider", "revision", "handle"); err != nil {
			return brokerControlResponse{}, err
		}
		if err := decodeBrokerExpectedProvider(fields, expectation.Provider, &response); err != nil {
			return brokerControlResponse{}, err
		}
		response.Revision, err = brokerRequiredField[string](fields, "revision")
		if err != nil || !validAuthRevision(response.Revision) {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker handle revision is invalid")
		}
		response.Handle, err = brokerRequiredField[string](fields, "handle")
		if err != nil || !validProjectHandle(response.Handle) {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker project handle is invalid")
		}
	case brokerControlBindingStatus:
		if err := requireBrokerFields(fields, "schema_version", "ok", "state", "provider", "revision"); err != nil {
			return brokerControlResponse{}, err
		}
		if err := decodeBrokerExpectedProvider(fields, expectation.Provider, &response); err != nil {
			return brokerControlResponse{}, err
		}
		response.Revision, err = brokerRequiredField[string](fields, "revision")
		if err != nil || response.Revision != expectation.Revision {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker binding status revision does not match the request")
		}
		response.State, err = brokerRequiredField[string](fields, "state")
		if err != nil || (response.State != "ready" && response.State != "missing" && response.State != "stale") {
			return brokerControlResponse{}, fmt.Errorf("Auth Broker binding status state is invalid")
		}
	default:
		return brokerControlResponse{}, fmt.Errorf("Auth Broker response operation is unsupported")
	}
	return response, nil
}

func decodeBrokerProviderState(
	fields map[string]json.RawMessage,
	expectation brokerControlExpectation,
	response *brokerControlResponse,
) error {
	if err := decodeBrokerExpectedProvider(fields, expectation.Provider, response); err != nil {
		return err
	}
	state, err := brokerRequiredField[string](fields, "state")
	if err != nil {
		return fmt.Errorf("Auth Broker provider state is invalid")
	}
	response.State = state
	return nil
}

func decodeBrokerExpectedProvider(
	fields map[string]json.RawMessage, expected string, response *brokerControlResponse,
) error {
	provider, err := brokerRequiredField[string](fields, "provider")
	if err != nil || provider != expected {
		return fmt.Errorf("Auth Broker response provider does not match the request")
	}
	response.Provider = provider
	return nil
}

func decodeBrokerCredentialMetadata(
	fields map[string]json.RawMessage, requireAccountLabel bool, response *brokerControlResponse,
) error {
	revision, err := brokerRequiredField[string](fields, "revision")
	if err != nil || !validAuthRevision(revision) {
		return fmt.Errorf("Auth Broker credential revision is invalid")
	}
	response.Revision = revision
	rawLabel, present := fields["account_label"]
	if !present {
		if requireAccountLabel {
			return fmt.Errorf("Auth Broker account label is missing")
		}
		return nil
	}
	labelFields := map[string]json.RawMessage{"account_label": rawLabel}
	label, err := brokerRequiredField[string](labelFields, "account_label")
	if err != nil || authbroker.ValidateSecretFreeText("account label", label, 128) != nil {
		return fmt.Errorf("Auth Broker account label is invalid")
	}
	response.AccountLabel = &label
	return nil
}

func decodeBrokerJSONObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("Auth Broker response must be one JSON object")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("Auth Broker response field name is invalid")
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("Auth Broker response field name is invalid")
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, fmt.Errorf("Auth Broker response contains duplicate field %q", key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("Auth Broker response field %q is invalid", key)
		}
		fields[key] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, fmt.Errorf("Auth Broker response object is incomplete")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("Auth Broker response contains trailing data")
	}
	return fields, nil
}

func brokerRequiredField[T any](fields map[string]json.RawMessage, name string) (T, error) {
	var zero T
	raw, present := fields[name]
	if !present {
		return zero, fmt.Errorf("Auth Broker response field %q is missing", name)
	}
	var value *T
	if err := decodeStrictJSON(raw, &value); err != nil || value == nil {
		return zero, fmt.Errorf("Auth Broker response field %q has the wrong type", name)
	}
	return *value, nil
}

func requireBrokerFields(fields map[string]json.RawMessage, expected ...string) error {
	if len(fields) != len(expected) {
		return fmt.Errorf("Auth Broker response has an unexpected field set")
	}
	for _, name := range expected {
		if _, present := fields[name]; !present {
			return fmt.Errorf("Auth Broker response field %q is missing", name)
		}
	}
	return nil
}

func validBrokerControlCode(value string) bool {
	if len(value) == 0 || len(value) > 96 {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' {
			continue
		}
		if index > 0 && ((character >= '0' && character <= '9') || character == '_') {
			continue
		}
		return false
	}
	return true
}
