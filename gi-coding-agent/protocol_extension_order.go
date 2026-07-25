package gicodingagent

import "sort"

// OrderSources projects the final resource-loader order onto registries that
// were populated across multiple load phases. Stable sorting preserves the
// registration order within each extension and leaves runtime-only sources at
// the end.
func (r *ProtocolExtensionRuntime) OrderSources(
	sources []ProtocolSourceInfo,
) {
	if r == nil {
		return
	}
	ranks := make(map[string]int, len(sources))
	for index, source := range sources {
		key := protocolSourceOrderKey(source)
		if key == "" {
			continue
		}
		if _, exists := ranks[key]; !exists {
			ranks[key] = index
		}
	}

	r.registryMu.Lock()
	stableSortProtocolSources(r.commands, ranks, func(value ProtocolCommandRegistration) ProtocolSourceInfo {
		return value.SourceInfo
	})
	for eventType, handlers := range r.handlers {
		stableSortProtocolSources(handlers, ranks, func(value protocolEventHandlerRegistration) ProtocolSourceInfo {
			return value.source
		})
		r.handlers[eventType] = handlers
	}
	stableSortProtocolSources(r.inputHandlers, ranks, func(value protocolInputHandlerRegistration) ProtocolSourceInfo {
		return value.source
	})
	stableSortProtocolSources(r.tools, ranks, func(value SDKTool) ProtocolSourceInfo {
		return value.SourceInfo
	})
	stableSortProtocolSources(r.messageRegistrations, ranks, func(value ProtocolMessageRendererRegistration) ProtocolSourceInfo {
		return value.SourceInfo
	})
	stableSortProtocolSources(r.entryRegistrations, ranks, func(value ProtocolEntryRendererRegistration) ProtocolSourceInfo {
		return value.SourceInfo
	})
	stableSortProtocolSources(r.toolRendererRegistrations, ranks, func(value ProtocolToolRendererRegistration) ProtocolSourceInfo {
		return value.SourceInfo
	})
	stableSortProtocolSources(r.flags, ranks, func(value ProtocolFlagRegistration) ProtocolSourceInfo {
		return value.SourceInfo
	})
	stableSortProtocolSources(r.shortcuts, ranks, func(value ProtocolShortcutRegistration) ProtocolSourceInfo {
		return value.SourceInfo
	})
	stableSortProtocolSources(r.autocomplete, ranks, func(value ProtocolAutocompleteProviderRegistration) ProtocolSourceInfo {
		return value.SourceInfo
	})
	stableSortProtocolSources(r.viewTreeMounts, ranks, func(value ProtocolViewTreeMountRegistration) ProtocolSourceInfo {
		return value.SourceInfo
	})
	r.rebuildMessageRenderersLocked()
	r.rebuildEntryRenderersLocked()
	r.rebuildToolRenderersLocked()
	visibleFlags := r.visibleFlagRegistrationsLocked()
	for _, flag := range visibleFlags {
		if _, hasCLIValue := r.cliFlagValues[flag.Name]; !hasCLIValue {
			if flag.Default == nil {
				delete(r.flagValues, flag.Name)
			} else {
				r.flagValues[flag.Name] = flag.Default
			}
		}
		r.applyCLIFlagValueToRegistrationLocked(flag)
	}
	r.registryMu.Unlock()

	affectedProviders := map[string]bool{}
	r.providerMu.Lock()
	stableSortProtocolSources(r.providerRegistrations, ranks, func(value protocolProviderRegistration) ProtocolSourceInfo {
		return value.source
	})
	stableSortProtocolSources(r.pendingProviders, ranks, func(value protocolProviderRegistration) ProtocolSourceInfo {
		return value.source
	})
	for _, registration := range r.providerRegistrations {
		affectedProviders[registration.name] = true
	}
	r.rebuildProviderMapsLocked()
	hasBoundProviders := r.modelRuntime != nil || r.modelRegistry != nil
	r.providerMu.Unlock()
	if hasBoundProviders {
		r.rebuildProviderState(affectedProviders)
	}

	r.notifyCommandsChanged()
	r.notifyAutocompleteProvidersChanged()
	r.notifyMessageRenderersChanged()
	r.applyViewTreeMounts()
	r.ApplyToSession(r.boundSession)
}

func protocolSourceOrderKey(source ProtocolSourceInfo) string {
	if source.Path != "" {
		return "path\x00" + source.Path
	}
	return protocolSourceInfoKey(source)
}

func stableSortProtocolSources[T any](
	values []T,
	ranks map[string]int,
	source func(T) ProtocolSourceInfo,
) {
	unknownRank := len(ranks)
	sort.SliceStable(values, func(left, right int) bool {
		leftRank, ok := ranks[protocolSourceOrderKey(source(values[left]))]
		if !ok {
			leftRank = unknownRank
		}
		rightRank, ok := ranks[protocolSourceOrderKey(source(values[right]))]
		if !ok {
			rightRank = unknownRank
		}
		return leftRank < rightRank
	})
}
