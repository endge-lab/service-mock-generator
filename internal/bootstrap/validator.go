package bootstrap

import validator "github.com/endge-lab/service-kit-go/pkg/validator"

func InitValidator() validator.Validator {
	return validator.NewValidator()
}
