package tool

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	ErrorKindValidation    ErrorKind = "validation"
	ErrorKindExecution     ErrorKind = "execution"
	ErrorKindConfiguration ErrorKind = "configuration"
)

// Error 는 Tool 구현이 입력 검증, 실행, 설정 오류를 공통 형태로 반환하기 위한 오류다.
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

func ValidationErrorf(format string, args ...any) error {
	return &Error{
		Kind:    ErrorKindValidation,
		Message: fmt.Sprintf(format, args...),
	}
}

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

func ConfigurationErrorf(format string, args ...any) error {
	return &Error{
		Kind:    ErrorKindConfiguration,
		Message: fmt.Sprintf(format, args...),
	}
}

func IsErrorKind(err error, kind ErrorKind) bool {
	var toolErr *Error
	return errors.As(err, &toolErr) && toolErr.Kind == kind
}

func IsValidationError(err error) bool {
	return IsErrorKind(err, ErrorKindValidation)
}

func IsExecutionError(err error) bool {
	return IsErrorKind(err, ErrorKindExecution)
}

func IsConfigurationError(err error) bool {
	return IsErrorKind(err, ErrorKindConfiguration)
}
