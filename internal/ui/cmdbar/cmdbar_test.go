package cmdbar_test

import (
	"strings"
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

func TestParseErrorsAreReadableAndListWhatIsValid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "a typo names the mistake and the alternatives",
			input: "stauts",
			want:  []string{`no command "stauts"`, "ns", "ctx", "sort", "filter"},
		},
		{
			name: "a filter expression points at the right key",
			// Reaching for ":" to type a filter is using the wrong key, not
			// making a mistake; the message should say which key.
			input: "stauts:age",
			want:  []string{"looks like a filter", "press / to filter"},
		},
		{
			name:  "an empty command lists the commands",
			input: "  ",
			want:  []string{"type a command", "ns", "ctx"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cmdbar.Parse(tt.input)
			if err == nil {
				t.Fatalf("Parse(%q) returned nil error", tt.input)
			}

			// These are read on a screen, not grepped in a log, so they must not
			// carry the internal error- prefix convention.
			if strings.HasPrefix(err.Error(), "error-") {
				t.Fatalf("user-facing message uses the log error prefix: %q", err)
			}

			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("Parse(%q) error %q missing %q", tt.input, err, want)
				}
			}
		})
	}
}

func TestParseAcceptsTheNewVerbsAndAliases(t *testing.T) {
	tests := []struct {
		input    string
		wantVerb string
		wantArg  string
	}{
		{input: "sort age", wantVerb: cmdbar.VerbSort, wantArg: "age"},
		{input: "filter status:crash", wantVerb: cmdbar.VerbFilter, wantArg: "status:crash"},
		{input: "q", wantVerb: cmdbar.VerbQuit},
		{input: "quit", wantVerb: cmdbar.VerbQuit},
		{input: "namespace kube-system", wantVerb: cmdbar.VerbNamespace, wantArg: "kube-system"},
		{input: "context", wantVerb: cmdbar.VerbContext},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := cmdbar.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.input, err)
			}

			if got.Verb != tt.wantVerb || got.Arg != tt.wantArg {
				t.Fatalf("Parse(%q) = %+v, want verb %q arg %q", tt.input, got, tt.wantVerb, tt.wantArg)
			}
		})
	}
}

func TestVerbsAreDocumentedWithTheirArguments(t *testing.T) {
	joined := strings.Join(cmdbar.Verbs(), " ")

	// The hint is what the user sees on an empty bar; a command missing from it
	// is undiscoverable.
	for _, want := range []string{"ns", "ctx", "sort", "filter", "q"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("Verbs() = %v, missing %q", cmdbar.Verbs(), want)
		}
	}
}
