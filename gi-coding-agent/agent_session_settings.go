package gicodingagent

// SyncRuntimeSettings refreshes the settings that affect AgentSession control
// flow. The SettingsManager remains the durable/live owner; AgentSession keeps
// only the per-run snapshot needed to make deterministic decisions.
func (s *AgentSession) SyncRuntimeSettings() {
	if s == nil || s.SettingsManager == nil {
		return
	}
	s.syncQueueModesFromSettings()
	s.CompactionSettings = s.SettingsManager.GetCompactionSettings()
	s.RetrySettings = s.SettingsManager.GetRetrySettings()
}

func (s *AgentSession) syncQueueModesFromSettings() {
	if s == nil || s.SettingsManager == nil {
		return
	}
	s.SteeringMode = s.SettingsManager.GetSteeringMode()
	s.FollowUpMode = s.SettingsManager.GetFollowUpMode()
}
