package cli

import "testing"

func TestMissingRunIndexWarning(t *testing.T) {
	tests := []struct {
		name     string
		producer string
		runIndex []byte
		want     bool
	}{
		{name: "indexed run", producer: "glyphrun", runIndex: []byte(`{}`)},
		{name: "ordinary artifact", producer: "fcheap"},
		{name: "monitor incident is not a run", producer: "monitor"},
		{name: "glyphrun without index", producer: "glyphrun", want: true},
		{name: "cairntrace without index", producer: "cairntrace", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := missingRunIndexWarning(test.producer, test.runIndex)
			if (got != "") != test.want {
				t.Fatalf("missingRunIndexWarning() = %q, want warning=%t", got, test.want)
			}
		})
	}
}
