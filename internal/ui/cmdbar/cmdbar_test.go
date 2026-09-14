package cmdbar_test

import (
	"testing"

	"github.com/mo-pankaj/kctl/internal/ui/cmdbar"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    cmdbar.Command
		wantErr bool
	}{
		{name: "namespace with argument", input: "ns trading-service", want: cmdbar.Command{Verb: "ns", Arg: "trading-service"}},
		{name: "leading colon is tolerated", input: ":ns dev-01", want: cmdbar.Command{Verb: "ns", Arg: "dev-01"}},
		{name: "extra whitespace is collapsed", input: "  ns    dev-01  ", want: cmdbar.Command{Verb: "ns", Arg: "dev-01"}},
		{name: "all namespaces", input: "ns all", want: cmdbar.Command{Verb: "ns", Arg: "all"}},
		{name: "verb with no argument", input: "ctx", want: cmdbar.Command{Verb: "ctx"}},
		{name: "verb is lowercased", input: "NS dev-01", want: cmdbar.Command{Verb: "ns", Arg: "dev-01"}},
		{name: "empty input is an error", input: "   ", wantErr: true},
		{name: "unknown verb is an error", input: "frobnicate x", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cmdbar.Parse(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) returned nil error, want an error", tt.input)
				}

				return
			}

			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tt.input, err)
			}

			if got != tt.want {
				t.Fatalf("Parse(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}
