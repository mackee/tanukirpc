package tanukirpc

import (
	"net/http"
	"reflect"
	"sync"

	"github.com/go-playground/validator/v10"
)

type Validatable interface {
	Validate() error
}

func canValidate(req any) (Validatable, bool) {
	v, ok := req.(Validatable)
	if ok {
		return v, true
	}
	if hasValidateTag(req) {
		return newStructValidator(req), true
	}
	return v, ok
}

type ValidateError struct {
	err error
}

func (v *ValidateError) Status() int {
	if ews, ok := v.err.(ErrorWithStatus); ok {
		return ews.Status()
	}
	return http.StatusBadRequest
}

func (v *ValidateError) Error() string {
	return v.err.Error()
}

func (v *ValidateError) Unwrap() error {
	return v.err
}

func hasValidateTag(req any) bool {
	v := reflect.ValueOf(req)
	t := v.Type()
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return false
	}
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < v.NumField(); i++ {
		fv := v.Field(i)
		ft := t.Field(i)
		ftt := ft.Type
		if ftt.Kind() == reflect.Pointer {
			ftt = ftt.Elem()
		}
		if ftt.Kind() == reflect.Struct {
			if hasValidateTag(fv.Interface()) {
				return true
			}
		}

		if _, ok := ft.Tag.Lookup("validate"); ok {
			return true
		}
	}
	return false
}

var defaultValidator = &sync.Pool{
	New: func() any {
		return validator.New(validator.WithRequiredStructEnabled())
	},
}

type structValidator struct {
	req any
	val *validator.Validate
}

func newStructValidator(req any) *structValidator {
	return &structValidator{req: req, val: defaultValidator.Get().(*validator.Validate)}
}

func (s *structValidator) Validate() error {
	return s.val.Struct(s.req)
}
