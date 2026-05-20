package gicodingagent

type TriggerCompactExtension struct {
	ThresholdTokens int
	seen            bool
	wasOver         bool
}

func NewTriggerCompactExtension(thresholdTokens int) *TriggerCompactExtension {
	return &TriggerCompactExtension{ThresholdTokens: thresholdTokens}
}

func (e *TriggerCompactExtension) OnTurnEnd(tokens *int, compact func()) {
	if e == nil || tokens == nil || e.ThresholdTokens <= 0 {
		return
	}
	over := *tokens >= e.ThresholdTokens
	if e.seen && !e.wasOver && over && compact != nil {
		compact()
	}
	e.seen = true
	e.wasOver = over
}
