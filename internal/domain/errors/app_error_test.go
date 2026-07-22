package errors

import (
	"errors"
	"testing"
)

func TestCodeOfReturnsSpecificWrappedCode(t *testing.T) {
	err := InvalidInput("validation.invalid_input", "Некорректные входные данные")

	if got := CodeOf(err); got != "validation.invalid_input" {
		t.Fatalf("CodeOf() = %q, want %q", got, "validation.invalid_input")
	}
	if got := HTTPStatusOf(err); got != 400 {
		t.Fatalf("HTTPStatusOf() = %d, want 400", got)
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatal("expected wrapped transport to match ErrInvalidInput")
	}
}

func TestWithDetailsPreservesAppError(t *testing.T) {
	err := WithDetails(ErrInvalidInput, map[string]any{"field": "identity"})

	if got := CodeOf(err); got != "common.invalid_input" {
		t.Fatalf("CodeOf() = %q, want %q", got, "common.invalid_input")
	}
	if got := DetailsOf(err)["field"]; got != "identity" {
		t.Fatalf("DetailsOf()[field] = %v, want identity", got)
	}
}
