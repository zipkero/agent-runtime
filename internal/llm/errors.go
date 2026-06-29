package llm

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	ErrorKindConfig   ErrorKind = "config"
	ErrorKindProvider ErrorKind = "provider"
	ErrorKindTimeout  ErrorKind = "timeout"
)

// Error 는 LLM 경계에서 설정 오류, provider 오류, timeout 오류를 구분한다.
type Error struct {
	Kind     ErrorKind
	Provider Provider
	Op       string
	Err      error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}

	if e.Provider == "" {
		return fmt.Sprintf("llm %s: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("llm %s %s: %v", e.Provider, e.Op, e.Err)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func IsKind(err error, kind ErrorKind) bool {
	var llmErr *Error
	if !errors.As(err, &llmErr) {
		return false
	}
	return llmErr.Kind == kind
}
