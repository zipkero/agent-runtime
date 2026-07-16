package llm

import (
	"errors"
	"fmt"
)

// ErrorKind 타입은 LLM 경계에서 호출자가 구분할 수 있는 실패 범주다.
type ErrorKind string

const (
	// ErrorKindConfig 상수는 공급자 선택이나 필수 설정 오류를 나타낸다.
	ErrorKindConfig ErrorKind = "config"
	// ErrorKindProvider 상수는 공급자 요청 또는 응답 처리 오류를 나타낸다.
	ErrorKindProvider ErrorKind = "provider"
	// ErrorKindTimeout 상수는 context deadline 또는 네트워크 제한 시간 초과를 나타낸다.
	ErrorKindTimeout ErrorKind = "timeout"
)

// Error 구조체는 LLM 경계에서 설정, 공급자, 제한 시간 초과 오류를 구분한다.
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

// IsKind 함수는 오류 체인에 지정한 LLM 오류 종류가 있는지 확인한다.
func IsKind(err error, kind ErrorKind) bool {
	var llmErr *Error
	if !errors.As(err, &llmErr) {
		return false
	}
	return llmErr.Kind == kind
}
