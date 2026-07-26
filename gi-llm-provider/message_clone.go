package gillmprovider

// cloneMessageState snapshots mutable message state before it crosses an
// asynchronous boundary. Message contains slices, maps, and pointers, so a
// plain struct assignment is not an event-time snapshot.
func cloneMessageState(message Message) Message {
	cloned := message
	cloned.Content = make([]ContentPart, len(message.Content))
	for index, part := range message.Content {
		cloned.Content[index] = part
		cloned.Content[index].Arguments = cloneCredentialMetadata(part.Arguments)
	}
	cloned.Diagnostics = make(
		[]AssistantMessageDiagnostic,
		len(message.Diagnostics),
	)
	for index, diagnostic := range message.Diagnostics {
		cloned.Diagnostics[index] = diagnostic
		if diagnostic.Error != nil {
			errorInfo := *diagnostic.Error
			errorInfo.Code = cloneCredentialMetadataValue(diagnostic.Error.Code)
			cloned.Diagnostics[index].Error = &errorInfo
		}
		cloned.Diagnostics[index].Details = cloneCredentialMetadata(diagnostic.Details)
	}
	if message.Display != nil {
		display := *message.Display
		cloned.Display = &display
	}
	cloned.Details = cloneCredentialMetadataValue(message.Details)
	cloned.AddedToolNames = append([]string(nil), message.AddedToolNames...)
	return cloned
}

// snapshotPartialEvents freezes one processor transition before the events are
// handed to another goroutine. Providers commonly emit several events from one
// wire chunk; Pi exposes the state after that whole chunk on each such event.
func snapshotPartialEvents(events []AssistantMessageEvent, state Message) []AssistantMessageEvent {
	for index := range events {
		events[index].Partial = cloneMessageState(state)
		events[index].ToolCall = cloneContentPartState(events[index].ToolCall)
		if events[index].Redacted != nil {
			redacted := *events[index].Redacted
			events[index].Redacted = &redacted
		}
	}
	return events
}

func cloneContentPartState(part ContentPart) ContentPart {
	cloned := part
	cloned.Arguments = cloneCredentialMetadata(part.Arguments)
	return cloned
}
