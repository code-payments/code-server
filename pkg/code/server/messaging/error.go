package messaging

import "fmt"

type MessageValidationError struct {
	message string
}

func newMessageValidationError(message string) MessageValidationError {
	return MessageValidationError{
		message: message,
	}
}

func newMessageValidationErrorf(format string, args ...any) MessageValidationError {
	return newMessageValidationError(fmt.Sprintf(format, args...))
}

func (e MessageValidationError) Error() string {
	return e.message
}

type MessageAuthenticationError struct {
	message string
}

func newMessageAuthenticationError(message string) MessageAuthenticationError {
	return MessageAuthenticationError{
		message: message,
	}
}

func newMessageAuthenticationErrorf(format string, args ...any) MessageAuthenticationError {
	return newMessageAuthenticationError(fmt.Sprintf(format, args...))
}

func (e MessageAuthenticationError) Error() string {
	return e.message
}

type MessageAuthorizationError struct {
	message string
}

func newMessageAuthorizationError(message string) MessageAuthorizationError {
	return MessageAuthorizationError{
		message: message,
	}
}

func newMessageAuthorizationErrorf(format string, args ...any) MessageAuthorizationError {
	return newMessageAuthorizationError(fmt.Sprintf(format, args...))
}

func (e MessageAuthorizationError) Error() string {
	return e.message
}
