package cmd

import (
	"strings"
	"testing"
)

func TestHandoffCommandHelpStatesReviewOnlyBoundary(t *testing.T) {
	help := strings.ToLower(handoffCmd.Short + "\n" + handoffCmd.Long)
	for _, required := range []string{"external agent", "never calls a model", "requires human review", "prepare", "verify"} {
		if !strings.Contains(help, required) {
			t.Fatalf("handoff help does not contain %q:\n%s", required, help)
		}
	}
}

func TestHandoffSubcommandsRequireAllInputs(t *testing.T) {
	for _, command := range []struct {
		name string
		run  func() error
	}{
		{name: "prepare", run: newHandoffPrepareCommand().ValidateRequiredFlags},
		{name: "verify", run: newHandoffVerifyCommand().ValidateRequiredFlags},
	} {
		t.Run(command.name, func(t *testing.T) {
			if err := command.run(); err == nil {
				t.Fatal("command without required flags succeeded")
			}
		})
	}
}
