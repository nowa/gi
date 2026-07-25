package gicodingagent

import "testing"

func TestAreExperimentalFeaturesEnabledPiCases(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "returns false when PI_EXPERIMENTAL is unset", env: map[string]string{}, want: false},
		{name: "returns false when PI_EXPERIMENTAL is empty", env: map[string]string{LegacyEnvExperimental: ""}, want: false},
		{name: "returns true when PI_EXPERIMENTAL is set to 1", env: map[string]string{LegacyEnvExperimental: "1"}, want: true},
		{name: "returns false when PI_EXPERIMENTAL is set to 0", env: map[string]string{LegacyEnvExperimental: "0"}, want: false},
		{name: "returns false when PI_EXPERIMENTAL is set to a non-1 value", env: map[string]string{LegacyEnvExperimental: "true"}, want: false},
		{name: "prefers the Gi experimental setting", env: map[string]string{
			EnvExperimental:       "0",
			LegacyEnvExperimental: "1",
		}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := experimentalFeaturesEnabled(func(key string) (string, bool) {
				value, ok := test.env[key]
				return value, ok
			})
			if got != test.want {
				t.Fatalf("experimentalFeaturesEnabled() = %t, want %t", got, test.want)
			}
		})
	}
}
