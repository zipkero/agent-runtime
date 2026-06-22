package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
)

// OutputContract 는 Runner가 최종 assistant text에 적용할 structured output 계약이다.
type OutputContract struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// StructuredOutputErrorKind 는 structured output 실패가 발생한 검증 단계를 분류한다.
type StructuredOutputErrorKind string

const (
	StructuredOutputErrorInvalidSchema StructuredOutputErrorKind = "invalid_schema"
	StructuredOutputErrorParse         StructuredOutputErrorKind = "parse"
	StructuredOutputErrorValidation    StructuredOutputErrorKind = "validation"
)

// StructuredOutputError 는 최종 text가 output contract를 만족하지 못한 원인을 호출자에게 전달한다.
type StructuredOutputError struct {
	ContractName string
	Kind         StructuredOutputErrorKind
	Err          error
}

func (e *StructuredOutputError) Error() string {
	if e.ContractName == "" {
		return fmt.Sprintf("structured output %s: %v", e.Kind, e.Err)
	}
	return fmt.Sprintf("structured output %q %s: %v", e.ContractName, e.Kind, e.Err)
}

func (e *StructuredOutputError) Unwrap() error {
	return e.Err
}

func cloneOutputContract(contract *OutputContract) *OutputContract {
	if contract == nil {
		return nil
	}
	clone := *contract
	clone.Schema = append(json.RawMessage(nil), contract.Schema...)
	return &clone
}

type outputInstructionMiddleware struct {
	contract OutputContract
}

func (m outputInstructionMiddleware) PreModel(_ context.Context, in PreModelInput) (llm.ChatRequest, error) {
	req := in.Request
	req.Messages = append([]message.Message{{
		Role:    message.RoleSystem,
		Content: []message.ContentBlock{message.NewTextBlock(outputInstructionText(m.contract))},
	}}, req.Messages...)
	return req, nil
}

func (m outputInstructionMiddleware) PostModel(_ context.Context, in PostModelInput) (llm.ChatResponse, error) {
	return in.Response, nil
}

func outputInstructionText(contract OutputContract) string {
	var b strings.Builder
	b.WriteString("You must produce the final assistant response as JSON only.\n")
	b.WriteString("Do not wrap the final JSON in Markdown or add explanatory text.\n")
	if contract.Name != "" {
		b.WriteString("Output contract name: ")
		b.WriteString(contract.Name)
		b.WriteString("\n")
	}
	if contract.Description != "" {
		b.WriteString("Output contract description: ")
		b.WriteString(contract.Description)
		b.WriteString("\n")
	}
	b.WriteString("JSON Schema:\n")
	var compact bytes.Buffer
	if err := json.Compact(&compact, contract.Schema); err == nil {
		b.Write(compact.Bytes())
	} else {
		b.Write(bytes.TrimSpace(contract.Schema))
	}
	return b.String()
}

func parseStructuredOutput(text string, contract OutputContract) (json.RawMessage, any, error) {
	schema, err := compileOutputSchema(contract)
	if err != nil {
		return nil, nil, err
	}

	raw := json.RawMessage(append([]byte(nil), text...))
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, nil, &StructuredOutputError{
			ContractName: contract.Name,
			Kind:         StructuredOutputErrorParse,
			Err:          err,
		}
	}
	validationValue, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, &StructuredOutputError{
			ContractName: contract.Name,
			Kind:         StructuredOutputErrorParse,
			Err:          err,
		}
	}
	if err := schema.Validate(validationValue); err != nil {
		return nil, nil, &StructuredOutputError{
			ContractName: contract.Name,
			Kind:         StructuredOutputErrorValidation,
			Err:          err,
		}
	}
	return raw, value, nil
}

func compileOutputSchema(contract OutputContract) (*jsonschema.Schema, error) {
	if len(bytes.TrimSpace(contract.Schema)) == 0 {
		return nil, &StructuredOutputError{
			ContractName: contract.Name,
			Kind:         StructuredOutputErrorInvalidSchema,
			Err:          errors.New("schema is empty"),
		}
	}

	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(contract.Schema))
	if err != nil {
		return nil, &StructuredOutputError{
			ContractName: contract.Name,
			Kind:         StructuredOutputErrorInvalidSchema,
			Err:          err,
		}
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", schemaDoc); err != nil {
		return nil, &StructuredOutputError{
			ContractName: contract.Name,
			Kind:         StructuredOutputErrorInvalidSchema,
			Err:          err,
		}
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, &StructuredOutputError{
			ContractName: contract.Name,
			Kind:         StructuredOutputErrorInvalidSchema,
			Err:          err,
		}
	}
	return schema, nil
}
