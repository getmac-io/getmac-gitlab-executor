package gitlab

type SystemFailureError struct {
	inner error
}

func NewSystemFailureError(inner error) error {
	return &SystemFailureError{inner: inner}
}

func (e *SystemFailureError) Error() string {
	return e.inner.Error()
}

func (e *SystemFailureError) Unwrap() error {
	return e.inner
}
