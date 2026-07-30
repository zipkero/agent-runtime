package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

const structuredOutputSchemaURL = "urn:agent-runtime:structured-output-schema"

type structuredOutputValidator struct {
	schema *jsonschema.Schema
}

func newStructuredOutputValidator(rawSchema json.RawMessage) (*structuredOutputValidator, error) {
	document, err := decodeJSONDocument(bytes.TrimSpace(rawSchema))
	if err != nil {
		return nil, structuredOutputError(StructuredOutputOperationSchemaCompile, err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(rejectExternalSchemaLoader{})
	if err := compiler.AddResource(structuredOutputSchemaURL, document); err != nil {
		return nil, structuredOutputError(StructuredOutputOperationSchemaCompile, err)
	}
	schema, err := compiler.Compile(structuredOutputSchemaURL)
	if err != nil {
		return nil, structuredOutputError(StructuredOutputOperationSchemaCompile, err)
	}
	return &structuredOutputValidator{schema: schema}, nil
}

func (v *structuredOutputValidator) Validate(text string) (json.RawMessage, error) {
	rawOutput := bytes.TrimSpace([]byte(text))
	document, err := decodeJSONDocument(rawOutput)
	if err != nil {
		return nil, structuredOutputError(StructuredOutputOperationJSONParse, err)
	}
	if err := v.schema.Validate(document); err != nil {
		return nil, structuredOutputError(StructuredOutputOperationValidation, err)
	}
	return append(json.RawMessage(nil), rawOutput...), nil
}

func decodeJSONDocument(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	// float64 변환으로 큰 정수의 정밀도가 깨지면 schema의 정수 제약 판정이 달라지므로 원문 숫자를 유지한다.
	decoder.UseNumber()

	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values are not allowed")
		}
		return nil, err
	}
	return document, nil
}

type rejectExternalSchemaLoader struct{}

func (rejectExternalSchemaLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema reference %q is not allowed", url)
}

func (s *AgentState) stopStructuredOutput(err error) {
	s.Status = StatusError
	s.FinalAnswer = ""
	s.LastError = err
	s.record(TraceActionStructuredOutputError, err)
}
