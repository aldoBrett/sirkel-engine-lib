// Package sirkel_errors carries a stable, machine-readable Code alongside a
// default Message on errors that are meaningful to show a user. The gate
// marshals Code/Message to the client; the frontend owns translating Code
// into a localized string, so Message is only an English fallback.
package sirkel_errors

// Code is a stable identifier a client can key an i18n lookup off of. Values
// are snake_case, matching the convention already used by
// tickets_repository.CheckInOutcome.
type Code string

// Error pairs a client-facing Code/Message with an optional server-side
// cause. Err is excluded from JSON (json:"-") and must never reach the
// client — it exists so callers can still chain errors.Is/errors.As through
// the wrapper and log the real underlying failure.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Err
}

// New builds an Error for a direct validation failure with no underlying
// cause to preserve.
func New(code Code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// Wrap builds an Error that also preserves the underlying cause, for
// unexpected failures that still need a code but whose real error should
// stay available for server-side logging/inspection.
func Wrap(code Code, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Err:     cause,
	}
}
