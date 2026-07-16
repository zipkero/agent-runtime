package tool

import (
	"errors"
	"fmt"
)

// ErrorKind 타입은 Tool 경계에서 호출자가 구분할 수 있는 실패 범주다.
type ErrorKind string

const (
	// ErrorKindValidation 상수는 Tool 입력 형식이나 값이 유효하지 않음을 나타낸다.
	ErrorKindValidation ErrorKind = "validation"
	// ErrorKindExecution 상수는 유효한 요청의 실행 중 발생한 실패를 나타낸다.
	ErrorKindExecution ErrorKind = "execution"
	// ErrorKindConfiguration 상수는 Tool 실행에 필요한 설정이 없거나 잘못되었음을 나타낸다.
	ErrorKindConfiguration ErrorKind = "configuration"
)

// Error 구조체는 Tool 구현이 입력 검증, 실행, 설정 오류를 공통 형태로 반환하기 위한 오류다.
type Error struct {
	Kind    ErrorKind
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Kind)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ValidationErrorf 함수는 입력 검증 실패를 나타내는 Tool 오류를 만든다.
func ValidationErrorf(format string, args ...any) error {
	return &Error{
		Kind:    ErrorKindValidation,
		Message: fmt.Sprintf(format, args...),
	}
}

// ExecutionErrorf 함수는 Tool 실행 실패를 나타내는 오류를 만든다.
func ExecutionErrorf(format string, args ...any) error {
	return &Error{
		Kind:    ErrorKindExecution,
		Message: fmt.Sprintf(format, args...),
	}
}

func canceledExecutionError(operation string, err error) error {
	return &Error{
		Kind:    ErrorKindExecution,
		Message: fmt.Sprintf("%s canceled: %v", operation, err),
		Err:     err,
	}
}

// ConfigurationErrorf 함수는 Tool 설정 실패를 나타내는 오류를 만든다.
func ConfigurationErrorf(format string, args ...any) error {
	return &Error{
		Kind:    ErrorKindConfiguration,
		Message: fmt.Sprintf(format, args...),
	}
}

// IsErrorKind 함수는 오류 체인에 지정한 Tool 오류 종류가 있는지 확인한다.
func IsErrorKind(err error, kind ErrorKind) bool {
	var toolErr *Error
	return errors.As(err, &toolErr) && toolErr.Kind == kind
}

// IsValidationError 함수는 오류 체인에 입력 검증 오류가 있는지 확인한다.
func IsValidationError(err error) bool {
	return IsErrorKind(err, ErrorKindValidation)
}

// IsExecutionError 함수는 오류 체인에 실행 오류가 있는지 확인한다.
func IsExecutionError(err error) bool {
	return IsErrorKind(err, ErrorKindExecution)
}

// IsConfigurationError 함수는 오류 체인에 설정 오류가 있는지 확인한다.
func IsConfigurationError(err error) bool {
	return IsErrorKind(err, ErrorKindConfiguration)
}
