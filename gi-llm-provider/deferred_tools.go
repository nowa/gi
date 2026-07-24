package gillmprovider

// ToolNameNormalizer maps provider-facing aliases to one comparison key.
type ToolNameNormalizer func(string) string

// DeferredToolSet is the request-time split between tools sent in the request
// prefix and tools loaded at transcript markers. Callers treat it as read-only
// after resolution.
type DeferredToolSet struct {
	Immediate     []Tool
	Deferred      map[string]Tool
	DeferredTools []Tool
}

// SplitDeferredTools classifies the current tool registry against transcript
// load markers. The last definition for a normalized name wins while the first
// occurrence owns deterministic output order, matching JavaScript Map order.
func SplitDeferredTools(context Context, enabled bool, normalizeName ToolNameNormalizer) DeferredToolSet {
	if normalizeName == nil {
		normalizeName = func(name string) string { return name }
	}

	order := make([]string, 0, len(context.Tools))
	uniqueTools := make(map[string]Tool, len(context.Tools))
	for _, tool := range context.Tools {
		name := normalizeName(tool.Name)
		if _, exists := uniqueTools[name]; !exists {
			order = append(order, name)
		}
		uniqueTools[name] = tool
	}
	if !enabled {
		immediate := make([]Tool, 0, len(order))
		for _, name := range order {
			immediate = append(immediate, uniqueTools[name])
		}
		return DeferredToolSet{Immediate: immediate, Deferred: map[string]Tool{}}
	}

	deferredNames := make(map[string]struct{})
	usedNames := make(map[string]struct{})
	for _, message := range context.Messages {
		switch message.Role {
		case RoleAssistant:
			for _, part := range message.Content {
				if part.Type == ContentToolCall {
					usedNames[normalizeName(part.Name)] = struct{}{}
				}
			}
		case RoleToolResult:
			for _, name := range message.AddedToolNames {
				normalized := normalizeName(name)
				if _, used := usedNames[normalized]; !used {
					deferredNames[normalized] = struct{}{}
				}
			}
		}
	}

	result := DeferredToolSet{
		Immediate:     make([]Tool, 0, len(order)),
		Deferred:      make(map[string]Tool, len(deferredNames)),
		DeferredTools: make([]Tool, 0, len(deferredNames)),
	}
	for _, name := range order {
		tool := uniqueTools[name]
		if _, deferred := deferredNames[name]; deferred {
			result.Deferred[name] = tool
			result.DeferredTools = append(result.DeferredTools, tool)
		} else {
			result.Immediate = append(result.Immediate, tool)
		}
	}
	return result
}

// splitKimiDeferredTools implements Kimi's system-message loading contract.
// Unlike tool-reference protocols, every schema named by a marker is removed
// from the request prefix, because the provider consumes the schema only from
// the system message emitted after that tool-result batch.
func splitKimiDeferredTools(context Context, enabled bool) DeferredToolSet {
	if !enabled {
		return DeferredToolSet{
			Immediate: append([]Tool(nil), context.Tools...),
			Deferred:  map[string]Tool{},
		}
	}

	deferredNames := getDeferredToolNames(context.Messages)

	result := DeferredToolSet{
		Immediate:     make([]Tool, 0, len(context.Tools)),
		Deferred:      make(map[string]Tool, len(deferredNames)),
		DeferredTools: make([]Tool, 0, len(deferredNames)),
	}
	deferredIndexes := make(map[string]int, len(deferredNames))
	for _, tool := range context.Tools {
		if _, deferred := deferredNames[tool.Name]; deferred {
			if index, exists := deferredIndexes[tool.Name]; exists {
				result.DeferredTools[index] = tool
			} else {
				deferredIndexes[tool.Name] = len(result.DeferredTools)
				result.DeferredTools = append(result.DeferredTools, tool)
			}
			result.Deferred[tool.Name] = tool
			continue
		}
		result.Immediate = append(result.Immediate, tool)
	}
	return result
}

func getDeferredToolNames(messages []Message) map[string]struct{} {
	names := make(map[string]struct{})
	for _, message := range messages {
		if message.Role != RoleToolResult {
			continue
		}
		for _, name := range message.AddedToolNames {
			names[name] = struct{}{}
		}
	}
	return names
}

func getToolsByName(tools map[string]Tool, names []string) []Tool {
	result := make([]Tool, 0, len(names))
	for _, name := range names {
		if tool, ok := tools[name]; ok {
			result = append(result, tool)
		}
	}
	return result
}
