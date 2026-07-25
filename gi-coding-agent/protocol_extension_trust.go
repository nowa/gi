package gicodingagent

import "fmt"

// ProtocolProjectTrustDecision is an extension's trust answer.
type ProtocolProjectTrustDecision string

const (
	// ProtocolProjectTrustYes trusts the project for the current runtime.
	ProtocolProjectTrustYes ProtocolProjectTrustDecision = "yes"
	// ProtocolProjectTrustNo rejects project-local settings and resources.
	ProtocolProjectTrustNo ProtocolProjectTrustDecision = "no"
	// ProtocolProjectTrustUndecided delegates to the next handler or fallback.
	ProtocolProjectTrustUndecided ProtocolProjectTrustDecision = "undecided"
)

// ProtocolProjectTrustResult is a decisive or fall-through extension result.
type ProtocolProjectTrustResult struct {
	Trusted  ProtocolProjectTrustDecision
	Remember bool
}

// ProtocolProjectTrustContext describes the startup surface available to a
// trusted in-process extension. UI functions are optional so print and RPC
// modes do not need a synthetic terminal implementation.
type ProtocolProjectTrustContext struct {
	CWD     string
	Mode    string
	HasUI   bool
	Select  func(prompt string, options []string) (string, error)
	Confirm func(prompt string) (bool, error)
	Input   func(prompt string) (string, error)
	// InputWithPlaceholder preserves Pi's two-field startup input contract
	// while Input remains the backward-compatible single-prompt form.
	InputWithPlaceholder func(title, placeholder string) (string, error)
	Notify               func(message string) error
}

// EmitProjectTrustEvent isolates handler failures and returns the first
// decisive answer. An undecided result deliberately falls through so later
// handlers and extensions can participate in the decision.
func (r *ProtocolExtensionRuntime) EmitProjectTrustEvent(
	context ProtocolProjectTrustContext,
) (*ProtocolProjectTrustResult, []ProtocolExtensionError) {
	if r == nil {
		return nil, nil
	}
	r.registryMu.RLock()
	handlers := append(
		[]protocolEventHandlerRegistration(nil),
		r.handlers[ProtocolEventProjectTrust]...,
	)
	r.registryMu.RUnlock()
	event := ProtocolSessionEvent{
		Type:                ProtocolEventProjectTrust,
		CWD:                 context.CWD,
		ProjectTrustContext: &context,
	}
	var errors []ProtocolExtensionError
	for _, registration := range handlers {
		result, err := invokeProtocolEventHandler(registration.handler, event)
		if err != nil {
			extensionError := ProtocolExtensionError{
				ExtensionPath: registration.source.Path,
				Event:         ProtocolEventProjectTrust,
				Error:         err.Error(),
			}
			errors = append(errors, extensionError)
			r.emitExtensionError(extensionError)
			continue
		}
		if result.ProjectTrust == nil ||
			result.ProjectTrust.Trusted == ProtocolProjectTrustUndecided ||
			result.ProjectTrust.Trusted == "" {
			continue
		}
		switch result.ProjectTrust.Trusted {
		case ProtocolProjectTrustYes, ProtocolProjectTrustNo:
			decision := *result.ProjectTrust
			return &decision, errors
		default:
			extensionError := ProtocolExtensionError{
				ExtensionPath: registration.source.Path,
				Event:         ProtocolEventProjectTrust,
				Error: fmt.Sprintf(
					"invalid project trust decision %q",
					result.ProjectTrust.Trusted,
				),
			}
			errors = append(errors, extensionError)
			r.emitExtensionError(extensionError)
		}
	}
	return nil, errors
}
