package gicodingagent

import "os"

const (
	EnvExperimental       = "GI_EXPERIMENTAL"
	LegacyEnvExperimental = "PI_EXPERIMENTAL"
)

// AreExperimentalFeaturesEnabled keeps experimental behavior opt-in. Gi's
// environment name takes precedence while the Pi name remains available for
// compatibility during migration.
func AreExperimentalFeaturesEnabled() bool {
	return experimentalFeaturesEnabled(os.LookupEnv)
}

func experimentalFeaturesEnabled(lookup func(string) (string, bool)) bool {
	if lookup == nil {
		return false
	}
	if value, ok := lookup(EnvExperimental); ok && value != "" {
		return value == "1"
	}
	value, _ := lookup(LegacyEnvExperimental)
	return value == "1"
}
