package app

import "testing"

func TestValidatePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		fallback string
		want     string
	}{
		{name: "empty uses fallback", input: "", fallback: "photos/", want: "photos/"},
		{name: "adds slash", input: "photos", fallback: "photos/", want: "photos/"},
		{name: "keeps slash", input: "photos/", fallback: "photos/", want: "photos/"},
		{name: "nested adds slash", input: "x/y", fallback: "photos/", want: "x/y/"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := validatePrefix(tt.input, tt.fallback); got != tt.want {
				t.Fatalf("validatePrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
