package portcheck

import "testing"

type samplePort struct{}

func TestIsNilDetectsNilInterfacesAndTypedNilValues(t *testing.T) {
	var pointer *samplePort
	var function func()
	var mapping map[string]string
	var slice []string
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "nil interface", want: true},
		{name: "typed nil pointer", value: pointer, want: true},
		{name: "typed nil function", value: function, want: true},
		{name: "typed nil map", value: mapping, want: true},
		{name: "typed nil slice", value: slice, want: true},
		{name: "value", value: samplePort{}, want: false},
		{name: "pointer", value: &samplePort{}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsNil(test.value); got != test.want {
				t.Fatalf("IsNil() = %t, want %t", got, test.want)
			}
		})
	}
}
