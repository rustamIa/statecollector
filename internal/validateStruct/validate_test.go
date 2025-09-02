// validate/validate_test.go
package validateStruct

import (
	"testing"
)

// -----------------------------------------------------------------------------
// ColumnsChecker

func TestColumnsChecker(t *testing.T) {
	cases := []struct {
		name  string
		line  []string
		count int
		want  bool
	}{
		{"exact columns", []string{"a", "b", "c"}, 3, true},
		{"less columns", []string{"a", "b"}, 3, false},
		{"more columns", []string{"a", "b", "c", "d"}, 3, false},
	}

	for _, tc := range cases {
		got := ColumnsChecker(tc.line, tc.count)
		if got != tc.want {
			t.Errorf("%s: want %v, got %v", tc.name, tc.want, got)
		}
	}
}

// -----------------------------------------------------------------------------
// Struct (валидатор)

type smsMock struct {
	Country  string `validate:"iso3166_1_alpha2"`
	Provider string `validate:"oneof=Topolo Rond Kildy"`
}

func TestStructValidation(t *testing.T) {
	tests := []struct {
		name string
		in   smsMock
		ok   bool
	}{
		{"valid", smsMock{Country: "US", Provider: "Topolo"}, true},
		{"bad country", smsMock{Country: "USA", Provider: "Topolo"}, false},
		{"bad provider", smsMock{Country: "US", Provider: "Foo"}, false},
	}

	for _, tt := range tests {
		err := Struct(tt.in)
		if (err == nil) != tt.ok {
			t.Errorf("%s: validation result mismatch; err=%v", tt.name, err)
		}
	}
}

// -----------------------------------------------------------------------------
// ValidateStruct (устаревшая обёртка — пока есть в коде)

func TestValidateStructWrapper(t *testing.T) {
	valid := smsMock{Country: "US", Provider: "Rond"}
	if ok, err := ValidateStruct(valid); !ok || err != nil {
		t.Fatalf("expected wrapper to pass on valid struct, got err=%v", err)
	}
}
