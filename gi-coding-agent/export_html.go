package gicodingagent

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const ExportHTMLTemplateCSS = `
:root {
  --body-bg: rgb(24, 24, 30);
  --container-bg: rgb(30, 30, 36);
  --info-bg: rgb(60, 55, 40);
  --text: #e6e6e6;
  --success: #b5bd68;
  --error: #cc6666;
  --warning: #ffff00;
  --muted: #808080;
  --dim: #666666;
  --accent: #8abeb7;
  --border: #5f87ff;
  --borderAccent: #00d7ff;
  --selectedBg: #3a3a4a;
  --customMessageLabel: #9575cd;
  --syntaxComment: #6A9955;
  --syntaxKeyword: #569CD6;
  --syntaxFunction: #DCDCAA;
  --syntaxString: #CE9178;
  --syntaxNumber: #B5CEA8;
  --syntaxType: #4EC9B0;
  --line-height: 18px;
  --sidebar-width: 400px;
  --sidebar-min-width: 240px;
  --sidebar-max-width: 840px;
  --sidebar-resizer-width: 6px;
}

* { box-sizing: border-box; }

body {
  margin: 0;
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 18px;
  color: var(--text);
  background: var(--body-bg);
}

body.sidebar-resizing {
  cursor: col-resize;
  user-select: none;
}

#app {
  display: flex;
  min-height: 100vh;
}

#sidebar {
  width: var(--sidebar-width);
  min-width: var(--sidebar-width);
  max-width: var(--sidebar-width);
  background: var(--container-bg);
  border-right: 1px solid var(--dim);
  height: 100vh;
  overflow: auto;
  position: sticky;
  top: 0;
  flex-shrink: 0;
}

#sidebar-resizer {
  width: var(--sidebar-resizer-width);
  flex-shrink: 0;
  position: sticky;
  top: 0;
  height: 100vh;
  cursor: col-resize;
  touch-action: none;
  background: transparent;
  border-right: 1px solid transparent;
}

#sidebar-resizer:hover,
body.sidebar-resizing #sidebar-resizer {
  background: var(--selectedBg);
  border-right-color: var(--dim);
}

.sidebar-header {
  padding: 8px 12px;
  border-bottom: 1px solid var(--dim);
}

.sidebar-controls {
  padding: 8px 8px 4px 8px;
}

.sidebar-search {
  width: 100%;
  padding: 4px 8px;
  font: inherit;
  font-size: 11px;
  color: var(--text);
  background: var(--body-bg);
  border: 1px solid var(--dim);
  border-radius: 3px;
}

.sidebar-search:focus {
  outline: none;
  border-color: var(--accent);
}

.sidebar-search::placeholder {
  color: var(--muted);
}

.sidebar-filters {
  display: flex;
  gap: 4px;
  align-items: center;
  flex-wrap: wrap;
  padding: 4px 8px 8px;
}

.filter-btn {
  padding: 3px 8px;
  font: inherit;
  font-size: 10px;
  color: var(--muted);
  background: transparent;
  border: 1px solid var(--dim);
  border-radius: 3px;
  cursor: pointer;
}

.filter-btn:hover {
  color: var(--text);
  border-color: var(--text);
}

.filter-btn.active {
  color: var(--body-bg);
  background: var(--accent);
  border-color: var(--accent);
}

.sidebar-close {
  display: none;
  margin-left: auto;
  padding: 3px 8px;
  font: inherit;
  font-size: 12px;
  color: var(--muted);
  background: transparent;
  border: 1px solid var(--dim);
  border-radius: 3px;
  cursor: pointer;
}

.sidebar-close:hover {
  color: var(--text);
  border-color: var(--text);
}

.tree-container {
  padding: 4px 0;
}

.tree-node {
  display: flex;
  gap: 6px;
  padding: 0 8px;
  font-size: 11px;
  line-height: 13px;
  color: var(--muted);
  text-decoration: none;
  white-space: nowrap;
}

.tree-node:hover,
.tree-node.active {
  background: var(--selectedBg);
  color: var(--text);
}

.tree-node.in-path {
  background: color-mix(in srgb, var(--accent) 10%, transparent);
}

.tree-node:not(.in-path) {
  opacity: 0.55;
}

.tree-node:not(.in-path):hover {
  opacity: 1;
}

.tree-marker {
  color: var(--accent);
  flex-shrink: 0;
}

.tree-role-user { color: var(--accent); }
.tree-role-assistant { color: var(--success); }
.tree-role-tool { color: var(--muted); }
.tree-role-custom { color: var(--customMessageLabel); }
.tree-role-skill { color: var(--customMessageLabel); }
.tree-role-compaction { color: var(--borderAccent); }
.tree-role-branch-summary { color: var(--warning); }

.tree-status {
  padding: 4px 12px;
  color: var(--muted);
  font-size: 11px;
  border-top: 1px solid var(--dim);
}

#content {
  flex: 1;
  min-width: 0;
  padding: 18px 36px;
}

#content > * {
  width: 100%;
  max-width: 900px;
  margin-left: auto;
  margin-right: auto;
}

.session-header {
  color: var(--muted);
  margin-bottom: 18px;
}

.header {
  margin-bottom: 18px;
}

.header h1 {
  margin: 0 0 8px;
  color: var(--text);
  font-size: 18px;
  line-height: 24px;
  font-weight: 700;
}

.header-info {
  display: flex;
  gap: 8px 16px;
  flex-wrap: wrap;
  color: var(--muted);
}

.info-item {
  display: flex;
  gap: 4px;
}

.info-label {
  color: var(--muted);
}

.info-value {
  color: var(--text);
}

.help-bar {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: center;
  margin: 8px 0;
}

.download-json-btn {
  padding: 3px 8px;
  font: inherit;
  font-size: 11px;
  color: var(--text);
  background: var(--container-bg);
  border: 1px solid var(--dim);
  border-radius: 3px;
  cursor: pointer;
}

.download-json-btn:hover {
  border-color: var(--accent);
}

.message {
  position: relative;
  margin: 0 0 18px;
  padding: 12px;
  background: var(--container-bg);
  border-radius: 4px;
}

.message-role {
  color: var(--muted);
  margin-bottom: 6px;
}

.message pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font: inherit;
}

.markdown-content p {
  margin: 0 0 8px;
}

.markdown-content p:last-child {
  margin-bottom: 0;
}

.markdown-content h1,
.markdown-content h2,
.markdown-content h3,
.markdown-content h4,
.markdown-content h5,
.markdown-content h6 {
  margin: 8px 0 4px;
  font-size: 1em;
}

.markdown-content ul,
.markdown-content ol {
  margin: 4px 0 8px 18px;
  padding: 0;
}

.markdown-content blockquote {
  margin: 4px 0 8px;
  padding-left: 10px;
  color: var(--muted);
  border-left: 2px solid var(--dim);
}

.markdown-content pre {
  margin: 6px 0;
  padding: 6px;
  overflow: auto;
  background: var(--body-bg);
  border: 1px solid var(--dim);
}

.markdown-content code {
  padding: 0 2px;
  background: var(--body-bg);
}

.markdown-content pre code {
  padding: 0;
  background: transparent;
}

.markdown-content a {
  color: var(--accent);
}

.hljs-comment,
.hljs-quote {
  color: var(--syntaxComment);
}

.hljs-keyword,
.hljs-selector-tag {
  color: var(--syntaxKeyword);
}

.hljs-number,
.hljs-literal {
  color: var(--syntaxNumber);
}

.hljs-string,
.hljs-doctag {
  color: var(--syntaxString);
}

.hljs-function,
.hljs-title,
.hljs-section,
.hljs-name {
  color: var(--syntaxFunction);
}

.hljs-type,
.hljs-class,
.hljs-built_in {
  color: var(--syntaxType);
}

.copy-link-btn {
  position: absolute;
  top: 6px;
  right: 6px;
  width: 24px;
  height: 24px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--muted);
  background: transparent;
  border: 1px solid transparent;
  border-radius: 3px;
  cursor: pointer;
  opacity: 0;
}

.message:hover .copy-link-btn,
.copy-link-btn:focus,
.copy-link-btn.copied {
  opacity: 1;
}

.copy-link-btn:hover,
.copy-link-btn.copied {
  color: var(--text);
  border-color: var(--dim);
  background: var(--body-bg);
}

.assistant-text,
.user-text,
.thinking-block,
.custom-message-content,
.tool-output {
  margin-top: 6px;
}

.tool-execution {
  margin-top: 8px;
  padding: 8px;
  border-left: 2px solid var(--dim);
  background: var(--body-bg);
}

.tool-execution.success {
  border-left-color: var(--success);
}

.tool-execution.error {
  border-left-color: var(--error);
}

.tool-execution.pending {
  border-left-color: var(--muted);
}

.tool-header {
  color: var(--muted);
  margin-bottom: 4px;
}

.tool-name {
  color: var(--text);
}

.tool-path {
  color: var(--muted);
}

.line-numbers,
.line-count {
  color: var(--muted);
}

.tool-command {
  color: var(--text);
}

.tool-error {
  color: var(--error);
}

.tool-output {
  color: var(--text);
}

.tool-output > div:not(.expand-hint),
.tool-output pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font: inherit;
}

.tool-output.expandable {
  cursor: pointer;
}

.tool-output.expandable .output-full {
  display: none;
}

.tool-output.expandable.expanded .output-preview {
  display: none;
}

.tool-output.expandable.expanded .output-full {
  display: block;
}

.expand-hint {
  color: var(--muted);
}

.tool-images {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 6px;
}

.tool-image {
  max-width: 320px;
  max-height: 240px;
  border: 1px solid var(--dim);
}

.tool-diff {
  margin-top: 6px;
  white-space: pre-wrap;
}

.ansi-rendered {
  white-space: pre-wrap;
}

.diff-added { color: var(--success); }
.diff-removed { color: var(--error); }
.diff-context { color: var(--muted); }

.thinking-block {
  color: var(--muted);
  font-style: italic;
}

.output-preview > div:not(.expand-hint),
.output-full > div:not(.expand-hint) {
  white-space: pre-wrap;
}

.ansi-line {
  white-space: pre;
}

#hamburger {
  display: none;
  position: fixed;
  top: 10px;
  left: 10px;
  z-index: 100;
  padding: 3px 8px;
  font: inherit;
  font-size: 12px;
  color: var(--muted);
  background: transparent;
  border: 1px solid var(--dim);
  border-radius: 3px;
  cursor: pointer;
}

#hamburger:hover {
  color: var(--text);
  border-color: var(--text);
}

#sidebar-overlay {
  display: none;
  position: fixed;
  inset: 0;
  z-index: 98;
  background: rgba(0, 0, 0, 0.5);
}

@media (max-width: 900px) {
  #sidebar {
    position: fixed;
    left: 0;
    top: 0;
    bottom: 0;
    width: min(var(--sidebar-width), 100vw);
    min-width: min(var(--sidebar-width), 100vw);
    max-width: min(var(--sidebar-width), 100vw);
    height: 100vh;
    z-index: 99;
    transform: translateX(-100%);
    transition: transform 0.3s;
  }

  #sidebar.open {
    transform: translateX(0);
  }

  #sidebar-resizer {
    display: none;
  }

  #sidebar-overlay.open {
    display: block;
  }

  #hamburger {
    display: block;
  }

  .sidebar-close {
    display: block;
  }

  #content {
    padding: var(--line-height) 16px;
  }

  #content > * {
    max-width: 100%;
  }
}

@media print {
  #sidebar,
  #sidebar-resizer,
  #hamburger,
  #sidebar-overlay {
    display: none !important;
  }

  body {
    color: black;
    background: white;
  }
}
`

type ExportHTMLOptions struct {
	ThemeName    string
	AgentDir     string
	ToolRenderer ExportHTMLToolRenderer
}

type ExportHTMLToolRenderer interface {
	RenderCall(toolCallID, toolName string, args any) string
	RenderResult(toolCallID, toolName string, result FileToolResult, isError bool) ExportHTMLToolRenderResult
}

type ExportHTMLToolRenderResult struct {
	Collapsed string
	Expanded  string
}

type ExportHTMLToolRendererDeps struct {
	GetToolDefinition func(name string) (ToolDefinition, bool)
	CWD               string
	Width             int
	ShowImages        bool
}

func CreateToolHTMLRenderer(deps ExportHTMLToolRendererDeps) ExportHTMLToolRenderer {
	width := deps.Width
	if width <= 0 {
		width = 100
	}
	return &exportHTMLToolRenderer{
		getToolDefinition: deps.GetToolDefinition,
		cwd:               deps.CWD,
		width:             width,
		showImages:        deps.ShowImages,
		states:            map[string]map[string]any{},
		args:              map[string]any{},
	}
}

type exportHTMLToolRenderer struct {
	getToolDefinition func(name string) (ToolDefinition, bool)
	cwd               string
	width             int
	showImages        bool
	states            map[string]map[string]any
	args              map[string]any
}

func (r *exportHTMLToolRenderer) RenderCall(toolCallID, toolName string, args any) string {
	if r == nil || r.getToolDefinition == nil {
		return ""
	}
	definition, ok := r.getToolDefinition(toolName)
	if !ok || definition.RenderCall == nil {
		return ""
	}
	r.args[toolCallID] = args
	lines, ok := safeRenderToolCall(definition.RenderCall, args, r.renderContext(toolCallID, false, true, false))
	if !ok {
		return ""
	}
	return AnsiLinesToHTML(lines)
}

func (r *exportHTMLToolRenderer) RenderResult(toolCallID, toolName string, result FileToolResult, isError bool) ExportHTMLToolRenderResult {
	if r == nil || r.getToolDefinition == nil {
		return ExportHTMLToolRenderResult{}
	}
	definition, ok := r.getToolDefinition(toolName)
	if !ok || definition.RenderResult == nil {
		return ExportHTMLToolRenderResult{}
	}
	collapsedLines, collapsedOK := safeRenderToolResult(definition.RenderResult, result, ToolRenderResultOptions{
		Expanded:  false,
		IsPartial: false,
	}, r.renderContext(toolCallID, false, false, isError))
	expandedLines, expandedOK := safeRenderToolResult(definition.RenderResult, result, ToolRenderResultOptions{
		Expanded:  true,
		IsPartial: false,
	}, r.renderContext(toolCallID, true, false, isError))
	rendered := ExportHTMLToolRenderResult{}
	if expandedOK {
		rendered.Expanded = RenderCustomToolResultHTML(expandedLines)
	}
	if collapsedOK {
		collapsed := RenderCustomToolResultHTML(collapsedLines)
		if collapsed != rendered.Expanded {
			rendered.Collapsed = collapsed
		}
	}
	return rendered
}

func (r *exportHTMLToolRenderer) renderContext(toolCallID string, expanded, isPartial, isError bool) ToolRenderContext {
	return ToolRenderContext{
		Args:             r.args[toolCallID],
		ToolCallID:       toolCallID,
		State:            r.state(toolCallID),
		CWD:              r.cwd,
		ArgsComplete:     true,
		IsPartial:        isPartial,
		Expanded:         expanded,
		ShowImages:       r.showImages,
		IsError:          isError,
		ExecutionStarted: true,
	}
}

func (r *exportHTMLToolRenderer) state(toolCallID string) map[string]any {
	state := r.states[toolCallID]
	if state == nil {
		state = map[string]any{}
		r.states[toolCallID] = state
	}
	return state
}

func ExportHTMLTemplateCSSWithOptions(options ExportHTMLOptions) string {
	themeVars := exportHTMLThemeVars(options)
	if strings.TrimSpace(themeVars) == "" {
		return ExportHTMLTemplateCSS
	}
	return ExportHTMLTemplateCSS + "\n:root {\n  " + strings.ReplaceAll(themeVars, "\n", "\n  ") + "\n}\n"
}

func exportHTMLThemeVars(options ExportHTMLOptions) string {
	colors, exportColors := exportHTMLThemeColors(options)
	if len(colors) == 0 {
		return ""
	}
	userMessageBg := colors["userMessageBg"]
	if strings.TrimSpace(userMessageBg) == "" {
		userMessageBg = "#343541"
	}
	derived := deriveExportHTMLColors(userMessageBg)
	pageBg := exportHTMLColorOr(exportColors.PageBg, derived.PageBg)
	cardBg := exportHTMLColorOr(exportColors.CardBg, derived.CardBg)
	infoBg := exportHTMLColorOr(exportColors.InfoBg, derived.InfoBg)

	keys := make([]string, 0, len(colors))
	for key := range colors {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		value := strings.TrimSpace(colors[key])
		if key == "" || value == "" {
			continue
		}
		builder.WriteString("--")
		builder.WriteString(key)
		builder.WriteString(": ")
		builder.WriteString(value)
		builder.WriteString(";\n")
	}
	for _, line := range []string{
		"--exportPageBg: " + pageBg + ";",
		"--exportCardBg: " + cardBg + ";",
		"--exportInfoBg: " + infoBg + ";",
		"--body-bg: " + pageBg + ";",
		"--container-bg: " + cardBg + ";",
		"--info-bg: " + infoBg + ";",
	} {
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func exportHTMLThemeColors(options ExportHTMLOptions) (map[string]string, ThemeExportColors) {
	themeName := strings.TrimSpace(options.ThemeName)
	if themeName == "" {
		themeName = "dark"
	}
	colors, err := GetResolvedThemeCSSColors(themeName, options.AgentDir)
	if err != nil || len(colors) == 0 {
		colors, _ = builtinThemeCSSColors("dark")
		themeName = "dark"
	}
	exportColors := builtinExportHTMLThemeColors(themeName)
	if exportColors.PageBg == nil && themeName != "dark" && themeName != "light" {
		if theme, err := loadThemeExportFile(themeName, options.AgentDir); err == nil {
			exportColors = themeExportColors(theme)
		}
	}
	return colors, exportColors
}

func builtinExportHTMLThemeColors(themeName string) ThemeExportColors {
	switch strings.TrimSpace(themeName) {
	case "light":
		return ThemeExportColors{
			PageBg: stringPtr("#f8f8f8"),
			CardBg: stringPtr("#ffffff"),
			InfoBg: stringPtr("#fffae6"),
		}
	case "", "dark":
		return ThemeExportColors{
			PageBg: stringPtr("#18181e"),
			CardBg: stringPtr("#1e1e24"),
			InfoBg: stringPtr("#3c3728"),
		}
	default:
		return ThemeExportColors{}
	}
}

func exportHTMLColorOr(value *string, fallback string) string {
	if value != nil && strings.TrimSpace(*value) != "" {
		return strings.TrimSpace(*value)
	}
	return fallback
}

type exportHTMLRGB struct {
	R int
	G int
	B int
}

type exportHTMLDerivedColors struct {
	PageBg string
	CardBg string
	InfoBg string
}

func parseExportHTMLColor(color string) (exportHTMLRGB, bool) {
	trimmed := strings.TrimSpace(color)
	if len(trimmed) == 7 && strings.HasPrefix(trimmed, "#") {
		r, errR := strconv.ParseInt(trimmed[1:3], 16, 0)
		g, errG := strconv.ParseInt(trimmed[3:5], 16, 0)
		b, errB := strconv.ParseInt(trimmed[5:7], 16, 0)
		if errR == nil && errG == nil && errB == nil {
			return exportHTMLRGB{R: int(r), G: int(g), B: int(b)}, true
		}
	}
	matches := regexp.MustCompile(`^rgb\s*\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)$`).FindStringSubmatch(trimmed)
	if matches != nil {
		r, errR := strconv.Atoi(matches[1])
		g, errG := strconv.Atoi(matches[2])
		b, errB := strconv.Atoi(matches[3])
		if errR == nil && errG == nil && errB == nil {
			return exportHTMLRGB{R: clampRGB(r), G: clampRGB(g), B: clampRGB(b)}, true
		}
	}
	return exportHTMLRGB{}, false
}

func exportHTMLLuminance(color exportHTMLRGB) float64 {
	toLinear := func(component int) float64 {
		s := float64(component) / 255.0
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*toLinear(color.R) + 0.7152*toLinear(color.G) + 0.0722*toLinear(color.B)
}

func adjustExportHTMLBrightness(color string, factor float64) string {
	parsed, ok := parseExportHTMLColor(color)
	if !ok {
		return color
	}
	adjust := func(component int) int {
		return clampRGB(int(math.Round(float64(component) * factor)))
	}
	return fmt.Sprintf("rgb(%d, %d, %d)", adjust(parsed.R), adjust(parsed.G), adjust(parsed.B))
}

func deriveExportHTMLColors(baseColor string) exportHTMLDerivedColors {
	parsed, ok := parseExportHTMLColor(baseColor)
	if !ok {
		return exportHTMLDerivedColors{
			PageBg: "rgb(24, 24, 30)",
			CardBg: "rgb(30, 30, 36)",
			InfoBg: "rgb(60, 55, 40)",
		}
	}
	if exportHTMLLuminance(parsed) > 0.5 {
		return exportHTMLDerivedColors{
			PageBg: adjustExportHTMLBrightness(baseColor, 0.96),
			CardBg: baseColor,
			InfoBg: fmt.Sprintf("rgb(%d, %d, %d)", clampRGB(parsed.R+10), clampRGB(parsed.G+5), clampRGB(parsed.B-20)),
		}
	}
	return exportHTMLDerivedColors{
		PageBg: adjustExportHTMLBrightness(baseColor, 0.7),
		CardBg: adjustExportHTMLBrightness(baseColor, 0.85),
		InfoBg: fmt.Sprintf("rgb(%d, %d, %d)", clampRGB(parsed.R+20), clampRGB(parsed.G+15), parsed.B),
	}
}

const ExportHTMLTemplateJS = `
(function() {
  'use strict';

  function decodeSessionData() {
    const element = document.getElementById('session-data');
    if (!element) return { entries: [] };
    const binary = atob((element.textContent || '').trim());
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    return JSON.parse(new TextDecoder('utf-8').decode(bytes));
  }

  function readURLParams() {
    const injected = document.querySelector('meta[name="pi-url-params"]');
    const search = injected ? injected.content : window.location.search.substring(1);
    return new URLSearchParams(search);
  }

  function buildEntryMap(entries) {
    const byId = new Map();
    for (const entry of entries || []) {
      if (entry && entry.id) byId.set(entry.id, entry);
    }
    return byId;
  }

  function buildChildrenMap(entries) {
    const children = new Map();
    for (const entry of entries || []) {
      if (!entry || !entry.id || entry.type === 'session') continue;
      const parentId = entry.parentId == null || entry.parentId === entry.id ? null : entry.parentId;
      if (!children.has(parentId)) children.set(parentId, []);
      children.get(parentId).push(entry);
    }
    children.forEach(function(list) {
      list.sort(function(a, b) {
        return new Date(a.timestamp || 0).getTime() - new Date(b.timestamp || 0).getTime();
      });
    });
    return children;
  }

  function buildActivePathIds(byId, targetId) {
    const ids = new Set();
    let current = byId.get(targetId);
    while (current) {
      ids.add(current.id);
      if (!current.parentId || current.parentId === current.id) break;
      current = byId.get(current.parentId);
    }
    return ids;
  }

  function getPath(byId, targetId) {
    const path = [];
    let current = byId.get(targetId);
    while (current) {
      path.unshift(current);
      if (!current.parentId || current.parentId === current.id) break;
      current = byId.get(current.parentId);
    }
    return path;
  }

  function findNewestLeaf(nodeId) {
    let currentId = nodeId;
    for (;;) {
      const next = childrenByParent.get(currentId);
      if (!next || next.length === 0) return currentId;
      currentId = next[next.length - 1].id;
    }
  }

  function formatTokens(count) {
    count = Number(count || 0);
    if (count < 1000) return String(count);
    if (count < 10000) return (count / 1000).toFixed(1) + 'k';
    if (count < 1000000) return Math.round(count / 1000) + 'k';
    return (count / 1000000).toFixed(1) + 'M';
  }

  function computeStats(entries) {
    const stats = {
      userMessages: 0,
      assistantMessages: 0,
      toolResults: 0,
      customMessages: 0,
      compactions: 0,
      branchSummaries: 0,
      toolCalls: 0,
      inputTokens: 0,
      outputTokens: 0,
      cacheReadTokens: 0,
      cacheWriteTokens: 0,
      models: new Set()
    };
    for (const entry of entries || []) {
      if (!entry) continue;
      if (entry.type === 'message' && entry.message) {
        const message = entry.message;
        if (message.role === 'user') stats.userMessages++;
        if (message.role === 'toolResult') stats.toolResults++;
        if (message.role === 'assistant') {
          stats.assistantMessages++;
          if (message.model) stats.models.add(message.provider ? message.provider + '/' + message.model : message.model);
          if (Array.isArray(message.content)) {
            stats.toolCalls += message.content.filter(function(part) {
              return part && part.type === 'toolCall';
            }).length;
          }
          if (message.usage) {
            stats.inputTokens += message.usage.input || 0;
            stats.outputTokens += message.usage.output || 0;
            stats.cacheReadTokens += message.usage.cacheRead || 0;
            stats.cacheWriteTokens += message.usage.cacheWrite || 0;
          }
        }
      } else if (entry.type === 'custom_message') {
        stats.customMessages++;
      } else if (entry.type === 'compaction') {
        stats.compactions++;
      } else if (entry.type === 'branch_summary') {
        stats.branchSummaries++;
      }
    }
    return stats;
  }

  function appendInfo(container, label, value) {
    const item = document.createElement('div');
    item.className = 'info-item';
    const labelElement = document.createElement('span');
    labelElement.className = 'info-label';
    labelElement.textContent = label + ':';
    const valueElement = document.createElement('span');
    valueElement.className = 'info-value';
    valueElement.textContent = value;
    item.appendChild(labelElement);
    item.appendChild(valueElement);
    container.appendChild(item);
  }

  function downloadSessionJson(data) {
    const lines = [];
    if (data.header) lines.push(JSON.stringify(data.header));
    for (const entry of data.entries || []) lines.push(JSON.stringify(entry));
    const blob = new Blob([lines.join('\n')], { type: 'application/x-ndjson' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = ((data.header && data.header.id) || 'session') + '.jsonl';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  }

  function buildShareURL(entryId) {
    const baseURLMeta = document.querySelector('meta[name="pi-share-base-url"]');
    const params = new URLSearchParams();
    params.set('leafId', currentLeafId || '');
    params.set('targetId', entryId || currentTargetId || currentLeafId || '');
    if (baseURLMeta && baseURLMeta.content) {
      const separator = baseURLMeta.content.includes('?') ? '&' : '?';
      return baseURLMeta.content + separator + params.toString();
    }
    const url = new URL(window.location.href);
    const gistId = Array.from(url.searchParams.keys()).find(function(key) {
      return !url.searchParams.get(key);
    });
    url.search = gistId ? '?' + gistId + '&' + params.toString() : '?' + params.toString();
    return url.toString();
  }

  async function copyToClipboard(text, button) {
    let success = false;
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(text);
        success = true;
      }
    } catch (error) {
      success = false;
    }
    if (!success) {
      try {
        const textarea = document.createElement('textarea');
        textarea.value = text;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        success = document.execCommand('copy');
        document.body.removeChild(textarea);
      } catch (error) {
        success = false;
      }
    }
    if (success && button) {
      const originalText = button.textContent;
      button.textContent = 'ok';
      button.classList.add('copied');
      setTimeout(function() {
        button.textContent = originalText;
        button.classList.remove('copied');
      }, 1500);
    }
  }

  function renderCopyLinkButton(section, entryId) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'copy-link-btn';
    button.dataset.entryId = entryId;
    button.title = 'Copy link to this message';
    button.setAttribute('aria-label', 'Copy link to this message');
    button.textContent = 'link';
    button.addEventListener('click', function(event) {
      event.preventDefault();
      event.stopPropagation();
      copyToClipboard(buildShareURL(entryId), button);
    });
    section.appendChild(button);
  }

  function appendPreBlock(parent, className, text) {
    if (!text) return;
    const wrapper = document.createElement('div');
    wrapper.className = className;
    const pre = document.createElement('pre');
    pre.textContent = text;
    wrapper.appendChild(pre);
    parent.appendChild(wrapper);
  }

  function appendTextElement(parent, tagName, className, text) {
    const element = document.createElement(tagName);
    if (className) element.className = className;
    element.textContent = text;
    parent.appendChild(element);
    return element;
  }

  function isSafeMarkdownURL(rawURL) {
    const normalized = String(rawURL || '').trim().toLowerCase().replace(/\0/g, '');
    return normalized !== '' && !normalized.startsWith('javascript:') && !normalized.startsWith('vbscript:') && !normalized.startsWith('data:');
  }

  function nextMarkdownMarker(text, start) {
    const tick = String.fromCharCode(96);
    const candidates = [
      text.indexOf(tick, start),
      text.indexOf('**', start),
      text.indexOf('[', start),
      text.indexOf('~~', start)
    ].filter(function(index) {
      return index >= 0;
    });
    return candidates.length === 0 ? text.length : Math.min.apply(Math, candidates);
  }

  function appendInlineMarkdown(parent, text) {
    text = String(text || '');
    const tick = String.fromCharCode(96);
    let index = 0;
    while (index < text.length) {
      if (text.startsWith(tick, index)) {
        const end = text.indexOf(tick, index + 1);
        if (end > index) {
          appendTextElement(parent, 'code', '', text.slice(index + 1, end));
          index = end + 1;
          continue;
        }
      }
      if (text.startsWith('**', index)) {
        const end = text.indexOf('**', index + 2);
        if (end > index + 2) {
          const strong = document.createElement('strong');
          appendInlineMarkdown(strong, text.slice(index + 2, end));
          parent.appendChild(strong);
          index = end + 2;
          continue;
        }
      }
      if (text.startsWith('~~', index)) {
        const end = text.indexOf('~~', index + 2);
        if (end > index + 2) {
          const deleted = document.createElement('del');
          appendInlineMarkdown(deleted, text.slice(index + 2, end));
          parent.appendChild(deleted);
          index = end + 2;
          continue;
        }
      }
      if (text[index] === '[') {
        const closeBracket = text.indexOf(']', index + 1);
        const openParen = closeBracket >= 0 ? text.indexOf('(', closeBracket + 1) : -1;
        const closeParen = openParen >= 0 ? text.indexOf(')', openParen + 1) : -1;
        if (closeBracket > index && openParen === closeBracket + 1 && closeParen > openParen) {
          const label = text.slice(index + 1, closeBracket);
          const href = text.slice(openParen + 1, closeParen).trim();
          if (isSafeMarkdownURL(href)) {
            const link = document.createElement('a');
            link.href = href;
            appendInlineMarkdown(link, label);
            parent.appendChild(link);
          } else {
            parent.appendChild(document.createTextNode(label));
          }
          index = closeParen + 1;
          continue;
        }
      }
      const next = nextMarkdownMarker(text, index + 1);
      parent.appendChild(document.createTextNode(text.slice(index, next)));
      index = next;
    }
  }

  function appendParagraph(container, lines) {
    if (lines.length === 0) return;
    const paragraph = document.createElement('p');
    lines.forEach(function(line, lineIndex) {
      if (lineIndex > 0) paragraph.appendChild(document.createElement('br'));
      appendInlineMarkdown(paragraph, line);
    });
    container.appendChild(paragraph);
  }

  const syntaxKeywordSets = {
    javascript: new Set(['async', 'await', 'break', 'case', 'catch', 'class', 'const', 'continue', 'default', 'else', 'export', 'extends', 'finally', 'for', 'function', 'if', 'import', 'let', 'new', 'return', 'switch', 'throw', 'try', 'var', 'while']),
    typescript: new Set(['async', 'await', 'break', 'case', 'catch', 'class', 'const', 'continue', 'default', 'else', 'enum', 'export', 'extends', 'finally', 'for', 'function', 'if', 'implements', 'import', 'interface', 'let', 'new', 'private', 'public', 'readonly', 'return', 'switch', 'throw', 'try', 'type', 'var', 'while']),
    go: new Set(['break', 'case', 'chan', 'const', 'continue', 'default', 'defer', 'else', 'fallthrough', 'for', 'func', 'go', 'goto', 'if', 'import', 'interface', 'map', 'package', 'range', 'return', 'select', 'struct', 'switch', 'type', 'var']),
    python: new Set(['and', 'as', 'async', 'await', 'break', 'class', 'continue', 'def', 'elif', 'else', 'except', 'finally', 'for', 'from', 'if', 'import', 'in', 'is', 'lambda', 'not', 'or', 'pass', 'raise', 'return', 'try', 'while', 'with', 'yield']),
    rust: new Set(['as', 'async', 'await', 'break', 'const', 'continue', 'crate', 'else', 'enum', 'extern', 'fn', 'for', 'if', 'impl', 'in', 'let', 'loop', 'match', 'mod', 'move', 'mut', 'pub', 'ref', 'return', 'self', 'static', 'struct', 'trait', 'type', 'unsafe', 'use', 'where', 'while']),
    bash: new Set(['case', 'do', 'done', 'elif', 'else', 'esac', 'fi', 'for', 'function', 'if', 'in', 'then', 'until', 'while'])
  };

  function normalizeHighlightLanguage(language) {
    language = String(language || '').toLowerCase();
    if (language === 'js' || language === 'jsx' || language === 'mjs' || language === 'cjs') return 'javascript';
    if (language === 'ts' || language === 'tsx') return 'typescript';
    if (language === 'py') return 'python';
    if (language === 'sh' || language === 'zsh' || language === 'shell') return 'bash';
    return language;
  }

  function getLanguageFromPath(filePath) {
    const ext = String(filePath || '').split('.').pop().toLowerCase();
    const map = {
      js: 'javascript', jsx: 'javascript', mjs: 'javascript', cjs: 'javascript',
      ts: 'typescript', tsx: 'typescript',
      go: 'go', py: 'python', rs: 'rust',
      sh: 'bash', bash: 'bash', zsh: 'bash',
      json: 'json', md: 'markdown'
    };
    return map[ext] || '';
  }

  function appendSyntaxSpan(parent, className, text) {
    const span = document.createElement('span');
    span.className = className;
    span.textContent = text;
    parent.appendChild(span);
  }

  function appendHighlightedText(parent, code, language) {
    const normalized = normalizeHighlightLanguage(language);
    const keywords = syntaxKeywordSets[normalized] || new Set();
    let index = 0;
    while (index < code.length) {
      const rest = code.slice(index);
      if (rest.startsWith('//') || ((normalized === 'python' || normalized === 'bash') && rest.startsWith('#'))) {
        const end = code.indexOf('\n', index);
        const next = end < 0 ? code.length : end;
        appendSyntaxSpan(parent, 'hljs-comment', code.slice(index, next));
        index = next;
        continue;
      }
      if (rest.startsWith('/*')) {
        const end = code.indexOf('*/', index + 2);
        const next = end < 0 ? code.length : end + 2;
        appendSyntaxSpan(parent, 'hljs-comment', code.slice(index, next));
        index = next;
        continue;
      }
      const char = code[index];
      if (char === '"' || char === "'") {
        let end = index + 1;
        let escaped = false;
        while (end < code.length) {
          const current = code[end];
          if (!escaped && current === char) {
            end++;
            break;
          }
          escaped = !escaped && current === '\\';
          if (current !== '\\') escaped = false;
          end++;
        }
        appendSyntaxSpan(parent, 'hljs-string', code.slice(index, end));
        index = end;
        continue;
      }
      if (/[0-9]/.test(char)) {
        const match = /^[0-9][0-9A-Fa-f_xX.]*/.exec(rest);
        appendSyntaxSpan(parent, 'hljs-number', match[0]);
        index += match[0].length;
        continue;
      }
      if (/[A-Za-z_]/.test(char)) {
        const match = /^[A-Za-z_][A-Za-z0-9_]*/.exec(rest);
        const word = match[0];
        if (keywords.has(word)) {
          appendSyntaxSpan(parent, 'hljs-keyword', word);
        } else if (/^[A-Z]/.test(word)) {
          appendSyntaxSpan(parent, 'hljs-type', word);
        } else {
          parent.appendChild(document.createTextNode(word));
        }
        index += word.length;
        continue;
      }
      parent.appendChild(document.createTextNode(char));
      index++;
    }
  }

  function appendHighlightedCode(parent, code, language) {
    const codeElement = document.createElement('code');
    codeElement.className = 'hljs';
    const normalized = normalizeHighlightLanguage(language);
    if (normalized) codeElement.dataset.language = normalized;
    appendHighlightedText(codeElement, String(code || ''), normalized);
    parent.appendChild(codeElement);
    return codeElement;
  }

  function appendMarkdownBlock(parent, className, text) {
    text = String(text || '');
    if (!text.trim()) return;
    const container = document.createElement('div');
    container.className = className ? className + ' markdown-content' : 'markdown-content';
    const lines = text.split('\n');
    let paragraph = [];
    let index = 0;

    function flushParagraph() {
      appendParagraph(container, paragraph);
      paragraph = [];
    }

    while (index < lines.length) {
      const line = lines[index];
      const trimmed = line.trim();
      const fence = String.fromCharCode(96).repeat(3);
      if (trimmed === '') {
        flushParagraph();
        index++;
        continue;
      }
      if (trimmed.startsWith(fence)) {
        flushParagraph();
        const language = trimmed.slice(fence.length).trim().split(/\s+/)[0] || '';
        const codeLines = [];
        index++;
        while (index < lines.length && !lines[index].trim().startsWith(fence)) {
          codeLines.push(lines[index]);
          index++;
        }
        if (index < lines.length) index++;
        const pre = document.createElement('pre');
        appendHighlightedCode(pre, codeLines.join('\n'), language);
        container.appendChild(pre);
        continue;
      }
      const heading = /^(#{1,6})\s+(.+)$/.exec(trimmed);
      if (heading) {
        flushParagraph();
        const level = Math.min(6, heading[1].length);
        const h = document.createElement('h' + level);
        appendInlineMarkdown(h, heading[2]);
        container.appendChild(h);
        index++;
        continue;
      }
      if (/^>\s?/.test(trimmed)) {
        flushParagraph();
        const quote = document.createElement('blockquote');
        while (index < lines.length && /^>\s?/.test(lines[index].trim())) {
          if (quote.childNodes.length > 0) quote.appendChild(document.createElement('br'));
          appendInlineMarkdown(quote, lines[index].trim().replace(/^>\s?/, ''));
          index++;
        }
        container.appendChild(quote);
        continue;
      }
      if (/^[-*+]\s+/.test(trimmed)) {
        flushParagraph();
        const list = document.createElement('ul');
        while (index < lines.length && /^[-*+]\s+/.test(lines[index].trim())) {
          const item = document.createElement('li');
          appendInlineMarkdown(item, lines[index].trim().replace(/^[-*+]\s+/, ''));
          list.appendChild(item);
          index++;
        }
        container.appendChild(list);
        continue;
      }
      paragraph.push(line);
      index++;
    }
    flushParagraph();
    parent.appendChild(container);
  }

  function str(value) {
    if (typeof value === 'string') return value;
    if (value == null) return '';
    return null;
  }

  function replaceTabs(text) {
    return String(text || '').replace(/\t/g, '   ');
  }

  function shortenPath(path) {
    if (typeof path !== 'string') return '';
    if (path.startsWith('/Users/')) {
      const parts = path.split('/');
      if (parts.length > 2) return '~' + path.slice(('/Users/' + parts[2]).length);
    }
    if (path.startsWith('/home/')) {
      const parts = path.split('/');
      if (parts.length > 2) return '~' + path.slice(('/home/' + parts[2]).length);
    }
    return path;
  }

  function appendLineList(parent, className, lines) {
    const container = document.createElement('div');
    container.className = className;
    for (const line of lines) {
      appendTextElement(container, 'div', '', replaceTabs(line));
    }
    parent.appendChild(container);
    return container;
  }

  function appendCodeOutput(parent, className, code, language) {
    const container = document.createElement('div');
    container.className = className;
    const pre = document.createElement('pre');
    appendHighlightedCode(pre, code, language);
    container.appendChild(pre);
    parent.appendChild(container);
    return container;
  }

  function appendExpandableOutput(parent, text, maxLines, language) {
    text = replaceTabs(text);
    if (!text) return;
    const lines = text.split('\n');
    if (lines.length <= maxLines) {
      if (language) {
        appendCodeOutput(parent, 'tool-output', text, language);
      } else {
        appendLineList(parent, 'tool-output', lines);
      }
      return;
    }
    const output = document.createElement('div');
    output.className = 'tool-output expandable';
    output.addEventListener('click', function() {
      if (window.getSelection && window.getSelection().toString()) return;
      output.classList.toggle('expanded');
    });
    let preview;
    if (language) {
      preview = appendCodeOutput(output, 'output-preview', lines.slice(0, maxLines).join('\n'), language);
      appendCodeOutput(output, 'output-full', text, language);
    } else {
      preview = appendLineList(output, 'output-preview', lines.slice(0, maxLines));
      appendLineList(output, 'output-full', lines);
    }
    appendTextElement(preview, 'div', 'expand-hint', '... (' + (lines.length - maxLines) + ' more lines)');
    parent.appendChild(output);
  }

  function appendToolHeader(parent, name, pathText, suffixText) {
    const header = document.createElement('div');
    header.className = 'tool-header';
    appendTextElement(header, 'span', 'tool-name', name);
    if (pathText !== undefined) {
      header.appendChild(document.createTextNode(' '));
      appendTextElement(header, 'span', 'tool-path', pathText);
    }
    if (suffixText) {
      header.appendChild(document.createTextNode(' '));
      appendTextElement(header, 'span', 'line-count', suffixText);
    }
    parent.appendChild(header);
    return header;
  }

  function getResultText(result) {
    if (!result) return '';
    return contentText(result.content);
  }

  function resultImages(result) {
    if (!result || !Array.isArray(result.content)) return [];
    return result.content.filter(function(part) {
      return part && part.type === 'image';
    });
  }

  function appendResultImages(parent, result) {
    const images = resultImages(result);
    if (images.length === 0) return;
    const container = document.createElement('div');
    container.className = 'tool-images';
    for (const image of images) {
      const img = document.createElement('img');
      img.className = 'tool-image';
      img.src = 'data:' + (image.mimeType || image.MIMEType || 'image/png') + ';base64,' + (image.data || '');
      container.appendChild(img);
    }
    parent.appendChild(container);
  }

  function appendTrustedHTML(parent, className, trustedHTML) {
    if (!trustedHTML) return null;
    const container = document.createElement('div');
    container.className = className;
    container.innerHTML = trustedHTML;
    parent.appendChild(container);
    return container;
  }

  function findToolResult(toolCallId) {
    for (const entry of data.entries || []) {
      if (entry && entry.type === 'message' && entry.message && entry.message.role === 'toolResult' && entry.message.toolCallId === toolCallId) {
        return entry.message;
      }
    }
    return null;
  }

  function renderToolCall(parent, call) {
    const result = findToolResult(call.id);
    const wrapper = document.createElement('div');
    wrapper.className = 'tool-execution ' + (result ? (result.isError ? 'error' : 'success') : 'pending');
    const args = call.arguments || {};
    const name = call.name || 'tool';

    switch (name) {
      case 'bash': {
        const command = str(args.command);
        appendTextElement(wrapper, 'div', 'tool-command', '$ ' + (command === null ? '[invalid arg]' : (command || '...')));
        if (result) appendExpandableOutput(wrapper, getResultText(result).trim(), 5);
        break;
      }
      case 'read': {
        const filePath = str(args.file_path ?? args.path);
        let pathText = filePath === null ? '[invalid arg]' : shortenPath(filePath || '');
        if (filePath !== null && (args.offset !== undefined || args.limit !== undefined)) {
          const startLine = args.offset ?? 1;
          const endLine = args.limit !== undefined ? startLine + args.limit - 1 : '';
          pathText += ':' + startLine + (endLine ? '-' + endLine : '');
        }
        appendToolHeader(wrapper, 'read', pathText, '');
        if (result) {
          appendResultImages(wrapper, result);
          appendExpandableOutput(wrapper, getResultText(result), 10, filePath ? getLanguageFromPath(filePath) : '');
        }
        break;
      }
      case 'write': {
        const filePath = str(args.file_path ?? args.path);
        const content = str(args.content);
        const suffix = content !== null && content ? '(' + content.split('\n').length + ' lines)' : '';
        appendToolHeader(wrapper, 'write', filePath === null ? '[invalid arg]' : shortenPath(filePath || ''), suffix);
        if (content === null) {
          appendTextElement(wrapper, 'div', 'tool-error', '[invalid content arg - expected string]');
        } else {
          appendExpandableOutput(wrapper, content, 10, filePath ? getLanguageFromPath(filePath) : '');
        }
        if (result) appendExpandableOutput(wrapper, getResultText(result).trim(), 5);
        break;
      }
      case 'edit': {
        const filePath = str(args.file_path ?? args.path);
        appendToolHeader(wrapper, 'edit', filePath === null ? '[invalid arg]' : shortenPath(filePath || ''), '');
        if (result && result.details && typeof result.details.diff === 'string') {
          const diff = document.createElement('div');
          diff.className = 'tool-diff';
          for (const line of result.details.diff.split('\n')) {
            const className = line.startsWith('+') ? 'diff-added' : (line.startsWith('-') ? 'diff-removed' : 'diff-context');
            appendTextElement(diff, 'div', className, replaceTabs(line));
          }
          wrapper.appendChild(diff);
        } else if (result) {
          appendExpandableOutput(wrapper, getResultText(result).trim(), 10);
        }
        break;
      }
      case 'ls': {
        const dirPath = str(args.path);
        const suffix = args.limit !== undefined ? '(limit ' + String(args.limit) + ')' : '';
        appendToolHeader(wrapper, 'ls', dirPath === null ? '[invalid arg]' : shortenPath(dirPath || '.'), suffix);
        if (result) appendExpandableOutput(wrapper, getResultText(result).trim(), 20);
        break;
      }
      default: {
        const rendered = renderedTools[call.id];
        if (rendered && (rendered.callHtml || rendered.resultHtmlCollapsed || rendered.resultHtmlExpanded)) {
          if (rendered.callHtml) {
            appendTrustedHTML(wrapper, 'tool-header ansi-rendered', rendered.callHtml);
          } else {
            appendToolHeader(wrapper, name, undefined, '');
          }
          const collapsed = rendered.resultHtmlCollapsed || rendered.resultHtmlExpanded || '';
          const expanded = rendered.resultHtmlExpanded || rendered.resultHtmlCollapsed || '';
          if (collapsed && expanded && collapsed !== expanded) {
            const output = document.createElement('div');
            output.className = 'tool-output expandable ansi-rendered';
            output.addEventListener('click', function() {
              if (window.getSelection && window.getSelection().toString()) return;
              output.classList.toggle('expanded');
            });
            appendTrustedHTML(output, 'output-preview', collapsed);
            appendTrustedHTML(output, 'output-full', expanded);
            wrapper.appendChild(output);
          } else {
            appendTrustedHTML(wrapper, 'tool-output ansi-rendered', expanded || collapsed);
          }
          if (result && !expanded && !collapsed) appendExpandableOutput(wrapper, getResultText(result), 10);
          break;
        }
        appendToolHeader(wrapper, name, undefined, '');
        appendPreBlock(wrapper, 'tool-output', JSON.stringify(args, null, 2));
        if (result) appendExpandableOutput(wrapper, getResultText(result), 10);
      }
    }
    parent.appendChild(wrapper);
  }

  function renderEntryContent(section, entry) {
    if (entry.type === 'message' && entry.message) {
      const message = entry.message;
      if (message.role === 'toolResult') return false;
      if (Array.isArray(message.content)) {
        for (const part of message.content) {
          if (!part) continue;
          if (part.type === 'text') {
            appendMarkdownBlock(section, message.role === 'user' ? 'user-text' : 'assistant-text', part.text || '');
          } else if (part.type === 'thinking') {
            appendPreBlock(section, 'thinking-block', part.thinking || '');
          } else if (part.type === 'toolCall') {
            renderToolCall(section, part);
          } else if (part.type === 'image') {
            appendPreBlock(section, 'assistant-text', '[image ' + (part.mimeType || '') + ']');
          }
        }
      } else {
        appendMarkdownBlock(section, message.role === 'user' ? 'user-text' : 'assistant-text', contentText(message.content));
      }
      if (message.stopReason === 'aborted') appendPreBlock(section, 'tool-output', 'Aborted');
      if (message.stopReason === 'error') appendPreBlock(section, 'tool-output', 'Error: ' + (message.errorMessage || 'Unknown error'));
      return true;
    }
    if (entry.type === 'custom_message') {
      appendMarkdownBlock(section, 'custom-message-content', typeof entry.content === 'string' ? entry.content : JSON.stringify(entry.content || ''));
      return true;
    }
    appendMarkdownBlock(section, 'assistant-text', entryText(entry));
    return true;
  }

  function renderHeader(data) {
    const headerContainer = document.getElementById('header-container');
    if (!headerContainer) return;
    headerContainer.textContent = '';
    headerContainer.className = 'session-header';

    const header = document.createElement('div');
    header.className = 'header';
    const title = document.createElement('h1');
    title.textContent = 'Session: ' + ((data.header && data.header.id) || 'unknown');
    header.appendChild(title);

    const helpBar = document.createElement('div');
    helpBar.className = 'help-bar';
    const hint = document.createElement('span');
    hint.className = 'help-hint';
    hint.textContent = 'Static session export';
    const download = document.createElement('button');
    download.type = 'button';
    download.className = 'download-json-btn';
    download.title = 'Download session as JSONL';
    download.textContent = 'Download JSONL';
    download.addEventListener('click', function() {
      downloadSessionJson(data);
    });
    helpBar.appendChild(hint);
    helpBar.appendChild(download);
    header.appendChild(helpBar);

    const stats = computeStats(data.entries || []);
    const info = document.createElement('div');
    info.className = 'header-info';
    appendInfo(info, 'Date', data.header && data.header.timestamp ? new Date(data.header.timestamp).toLocaleString() : 'unknown');
    appendInfo(info, 'CWD', data.header && data.header.cwd ? data.header.cwd : 'unknown');
    appendInfo(info, 'Models', Array.from(stats.models).join(', ') || 'unknown');
    appendInfo(info, 'Messages', [
      stats.userMessages ? stats.userMessages + ' user' : '',
      stats.assistantMessages ? stats.assistantMessages + ' assistant' : '',
      stats.toolResults ? stats.toolResults + ' tool results' : '',
      stats.customMessages ? stats.customMessages + ' custom' : '',
      stats.compactions ? stats.compactions + ' compactions' : '',
      stats.branchSummaries ? stats.branchSummaries + ' branch summaries' : ''
    ].filter(Boolean).join(', ') || '0');
    appendInfo(info, 'Tool Calls', String(stats.toolCalls));
    appendInfo(info, 'Tokens', [
      stats.inputTokens ? 'up ' + formatTokens(stats.inputTokens) : '',
      stats.outputTokens ? 'down ' + formatTokens(stats.outputTokens) : '',
      stats.cacheReadTokens ? 'R' + formatTokens(stats.cacheReadTokens) : '',
      stats.cacheWriteTokens ? 'W' + formatTokens(stats.cacheWriteTokens) : ''
    ].filter(Boolean).join(' ') || '0');
    header.appendChild(info);
    headerContainer.appendChild(header);
  }

  function entryRole(entry) {
    if (!entry) return 'unknown';
    if (entry.type === 'message' && entry.message && entry.message.role) return entry.message.role;
    if (entry.type === 'custom_message') return entry.customType || 'custom';
    return entry.type || 'unknown';
  }

  function contentText(content) {
    if (typeof content === 'string') return content;
    if (!Array.isArray(content)) return '';
    return content.map(function(part) {
      if (!part) return '';
      if (part.type === 'text') return part.text || '';
      if (part.type === 'thinking') return part.thinking || '';
      if (part.type === 'toolCall') return '[tool call ' + (part.name || '') + '] ' + JSON.stringify(part.arguments || {});
      if (part.type === 'image') return '[image ' + (part.mimeType || '') + ']';
      return '';
    }).filter(Boolean).join('\n');
  }

  function entryText(entry) {
    if (!entry) return '';
    if (entry.type === 'message' && entry.message) return contentText(entry.message.content);
    if (entry.type === 'custom_message') return typeof entry.content === 'string' ? entry.content : JSON.stringify(entry.content || '');
    if (entry.summary) return entry.summary;
    if (entry.label) return entry.label;
    if (entry.name) return entry.name;
    return '';
  }

  function buildLabelMap(entries) {
    const labels = new Map();
    for (const entry of entries || []) {
      if (entry && entry.type === 'label' && entry.targetId && entry.label) {
        labels.set(entry.targetId, entry.label);
      }
    }
    return labels;
  }

  function hasTextContent(content) {
    if (typeof content === 'string') return content.trim().length > 0;
    if (!Array.isArray(content)) return false;
    return content.some(function(part) {
      return part && part.type === 'text' && part.text && part.text.trim().length > 0;
    });
  }

  function normalizedRoleClass(entry) {
    return entryRole(entry).replace(/_/g, '-').replace(/[^a-z0-9_-]/gi, '-');
  }

  function isSettingsEntry(entry) {
    return ['label', 'custom', 'model_change', 'thinking_level_change'].includes(entry.type);
  }

  function searchableText(entry, label) {
    const parts = [];
    if (label) parts.push(label);
    if (!entry) return '';
    parts.push(entry.type || '');
    if (entry.type === 'message' && entry.message) {
      parts.push(entry.message.role || '');
      parts.push(contentText(entry.message.content));
      if (entry.message.command) parts.push(entry.message.command);
    } else if (entry.type === 'custom_message') {
      parts.push(entry.customType || '');
      parts.push(entryText(entry));
    } else {
      parts.push(entryText(entry));
      if (entry.provider) parts.push(entry.provider);
      if (entry.modelId) parts.push(entry.modelId);
      if (entry.thinkingLevel) parts.push(entry.thinkingLevel);
    }
    return parts.join(' ').toLowerCase();
  }

  function passesFilter(entry, leafId) {
    if (!entry || entry.type === 'session') return false;
    if (entry.id === leafId) return true;
    if (entry.type === 'message' && entry.message && entry.message.role === 'assistant') {
      const hasText = hasTextContent(entry.message.content);
      const isErrorOrAborted = entry.message.stopReason && entry.message.stopReason !== 'stop' && entry.message.stopReason !== 'toolUse';
      if (!hasText && !isErrorOrAborted) return false;
    }

    const label = labelMap.get(entry.id);
    let allowed = true;
    switch (filterMode) {
      case 'user-only':
        allowed = entry.type === 'message' && entry.message && entry.message.role === 'user';
        break;
      case 'no-tools':
        allowed = !isSettingsEntry(entry) && !(entry.type === 'message' && entry.message && entry.message.role === 'toolResult');
        break;
      case 'labeled-only':
        allowed = label !== undefined;
        break;
      case 'all':
        allowed = true;
        break;
      default:
        allowed = !isSettingsEntry(entry);
        break;
    }
    if (!allowed) return false;

    const tokens = searchQuery.toLowerCase().split(/\s+/).filter(Boolean);
    if (tokens.length === 0) return true;
    const text = searchableText(entry, label);
    return tokens.every(function(token) {
      return text.includes(token);
    });
  }

  function filteredEntries(entries, leafId) {
    return (entries || []).filter(function(entry) {
      return passesFilter(entry, leafId);
    });
  }

  function renderTree(entries, leafId) {
    const tree = document.getElementById('tree-container');
    if (!tree) return;
    const visibleEntries = filteredEntries(entries, leafId);
    const activePathIds = buildActivePathIds(entriesById, leafId);
    tree.textContent = '';
    for (const entry of visibleEntries) {
      const link = document.createElement('a');
      link.className = 'tree-node';
      if (activePathIds.has(entry.id)) link.classList.add('in-path');
      if (entry.id === currentTargetId) link.classList.add('active');
      link.href = '#entry-' + encodeURIComponent(entry.id);
      link.dataset.id = entry.id;
      link.addEventListener('click', function(event) {
        event.preventDefault();
        if (window.getSelection && window.getSelection().toString()) return;
        navigateTo(findNewestLeaf(entry.id), entry.id);
      });
      const marker = document.createElement('span');
      marker.className = 'tree-marker';
      marker.textContent = activePathIds.has(entry.id) ? '*' : ' ';
      const role = document.createElement('span');
      role.className = 'tree-role-' + normalizedRoleClass(entry);
      role.textContent = entryRole(entry);
      const label = document.createElement('span');
      const text = (labelMap.get(entry.id) || entryText(entry)).replace(/\s+/g, ' ').trim();
      label.textContent = text.length > 72 ? text.slice(0, 69) + '...' : text;
      link.appendChild(marker);
      link.appendChild(role);
      link.appendChild(label);
      tree.appendChild(link);
    }
    const status = document.getElementById('tree-status');
    if (status) {
      status.textContent = visibleEntries.length + ' of ' + (entries || []).filter(function(entry) {
        return entry && entry.type !== 'session';
      }).length + ' entries';
    }
    setTimeout(function() {
      const active = tree.querySelector('.tree-node.active');
      if (active) active.scrollIntoView({ block: 'nearest' });
    }, 0);
  }

  function renderMessages(data) {
    renderHeader(data);
    const messages = document.getElementById('messages');
    if (!messages) return;
    messages.textContent = '';
    const path = getPath(entriesById, currentLeafId);
    const entries = path.length > 0 ? path : (data.entries || []);
    for (const entry of entries) {
      if (!entry || entry.type === 'session') continue;
      if (entry.type === 'message' && entry.message && entry.message.role === 'toolResult') continue;
      const section = document.createElement('section');
      section.className = 'message';
      section.id = 'entry-' + entry.id;
      section.dataset.entryId = entry.id;
      section.dataset.role = entryRole(entry);
      renderCopyLinkButton(section, entry.id);
      const role = document.createElement('div');
      role.className = 'message-role';
      role.textContent = entryRole(entry);
      section.appendChild(role);
      if (!renderEntryContent(section, entry)) continue;
      messages.appendChild(section);
    }
    setTimeout(function() {
      const target = document.getElementById('entry-' + currentTargetId);
      if (target) target.scrollIntoView({ block: 'nearest' });
    }, 0);
  }

  function updateURL() {
    if (!window.history || !window.history.replaceState) return;
    const params = new URLSearchParams(window.location.search.substring(1));
    params.set('leafId', currentLeafId || '');
    params.set('targetId', currentTargetId || currentLeafId || '');
    const nextURL = window.location.pathname + '?' + params.toString() + window.location.hash;
    window.history.replaceState(null, '', nextURL);
  }

  function navigateTo(leafId, targetId) {
    currentLeafId = entriesById.has(leafId) ? leafId : defaultLeafId;
    currentTargetId = entriesById.has(targetId) ? targetId : currentLeafId;
    updateURL();
    renderTree(data.entries || [], currentLeafId);
    renderMessages(data);
  }

  function setupSidebarControls(data) {
    const searchInput = document.getElementById('tree-search');
    if (searchInput) {
      searchInput.addEventListener('input', function(event) {
        searchQuery = event.target.value || '';
        renderTree(data.entries || [], currentLeafId);
      });
    }
    const buttons = document.querySelectorAll('.filter-btn[data-filter]');
    buttons.forEach(function(button) {
      button.addEventListener('click', function() {
        filterMode = button.dataset.filter || 'default';
        buttons.forEach(function(other) {
          other.classList.toggle('active', other === button);
        });
        renderTree(data.entries || [], currentLeafId);
      });
    });
  }

  function setupSidebarResize() {
    const sidebar = document.getElementById('sidebar');
    const resizer = document.getElementById('sidebar-resizer');
    if (!sidebar || !resizer) return;
    const storageKey = 'gi-share:v1:sidebar-width';
    const minContentWidth = 320;

    function isMobileLayout() {
      return window.matchMedia && window.matchMedia('(max-width: 900px)').matches;
    }

    function sidebarBounds() {
      const rootStyles = getComputedStyle(document.documentElement);
      const minWidth = parseFloat(rootStyles.getPropertyValue('--sidebar-min-width')) || 240;
      const maxWidth = parseFloat(rootStyles.getPropertyValue('--sidebar-max-width')) || 840;
      const viewportMaxWidth = window.innerWidth - minContentWidth;
      return {
        minWidth: minWidth,
        maxWidth: Math.max(minWidth, Math.min(maxWidth, viewportMaxWidth))
      };
    }

    function clampSidebarWidth(width) {
      const bounds = sidebarBounds();
      return Math.max(bounds.minWidth, Math.min(bounds.maxWidth, width));
    }

    function applySidebarWidth(width) {
      document.documentElement.style.setProperty('--sidebar-width', Math.round(clampSidebarWidth(width)) + 'px');
    }

    function loadSidebarWidth() {
      try {
        const raw = localStorage.getItem(storageKey);
        if (raw === null) return null;
        const width = Number(raw);
        return Number.isFinite(width) ? width : null;
      } catch (error) {
        return null;
      }
    }

    function saveSidebarWidth(width) {
      try {
        localStorage.setItem(storageKey, String(Math.round(clampSidebarWidth(width))));
      } catch (error) {
        // Ignore storage failures.
      }
    }

    const savedWidth = loadSidebarWidth();
    if (savedWidth !== null) applySidebarWidth(savedWidth);

    let cleanupDrag = null;
    function stopDrag(pointerId) {
      if (!cleanupDrag) return;
      cleanupDrag(pointerId);
      cleanupDrag = null;
    }

    resizer.addEventListener('pointerdown', function(event) {
      if (isMobileLayout()) return;
      event.preventDefault();
      const startX = event.clientX;
      const startWidth = sidebar.getBoundingClientRect().width;
      document.body.classList.add('sidebar-resizing');
      if (resizer.setPointerCapture) resizer.setPointerCapture(event.pointerId);

      function onPointerMove(moveEvent) {
        applySidebarWidth(startWidth + (moveEvent.clientX - startX));
      }

      function onPointerUp(upEvent) {
        stopDrag(upEvent.pointerId);
      }

      function onPointerCancel(cancelEvent) {
        stopDrag(cancelEvent.pointerId);
      }

      cleanupDrag = function(pointerIdToRelease) {
        document.body.classList.remove('sidebar-resizing');
        if (resizer.releasePointerCapture) resizer.releasePointerCapture(pointerIdToRelease);
        window.removeEventListener('pointermove', onPointerMove);
        window.removeEventListener('pointerup', onPointerUp);
        window.removeEventListener('pointercancel', onPointerCancel);
        saveSidebarWidth(sidebar.getBoundingClientRect().width);
      };

      window.addEventListener('pointermove', onPointerMove);
      window.addEventListener('pointerup', onPointerUp);
      window.addEventListener('pointercancel', onPointerCancel);
    });

    resizer.addEventListener('dblclick', function() {
      if (isMobileLayout()) return;
      applySidebarWidth(400);
      saveSidebarWidth(400);
    });

    window.addEventListener('resize', function() {
      if (isMobileLayout()) return;
      applySidebarWidth(sidebar.getBoundingClientRect().width);
    });
  }

  function setupMobileSidebar() {
    const sidebar = document.getElementById('sidebar');
    const overlay = document.getElementById('sidebar-overlay');
    const hamburger = document.getElementById('hamburger');
    const closeButton = document.getElementById('sidebar-close');
    if (!sidebar || !overlay || !hamburger || !closeButton) return;

    function openSidebar() {
      sidebar.classList.add('open');
      overlay.classList.add('open');
      hamburger.style.display = 'none';
    }

    function closeSidebar() {
      sidebar.classList.remove('open');
      overlay.classList.remove('open');
      hamburger.style.display = '';
    }

    hamburger.addEventListener('click', openSidebar);
    overlay.addEventListener('click', closeSidebar);
    closeButton.addEventListener('click', closeSidebar);
  }

  const data = decodeSessionData();
  const renderedTools = data.renderedTools || {};
  const entriesById = buildEntryMap(data.entries || []);
  const childrenByParent = buildChildrenMap(data.entries || []);
  const params = readURLParams();
  const defaultLeafId = data.leafId && entriesById.has(data.leafId) ? data.leafId : ((data.entries || []).filter(function(entry) {
    return entry && entry.id && entry.type !== 'session';
  }).slice(-1)[0] || {}).id;
  let currentLeafId = params.get('leafId') && entriesById.has(params.get('leafId')) ? params.get('leafId') : defaultLeafId;
  let currentTargetId = params.get('targetId') && entriesById.has(params.get('targetId')) ? params.get('targetId') : currentLeafId;
  const labelMap = buildLabelMap(data.entries || []);
  let filterMode = 'default';
  let searchQuery = '';
  window.downloadSessionJson = function() {
    downloadSessionJson(data);
  };
  setupSidebarControls(data);
  setupSidebarResize();
  setupMobileSidebar();
  renderTree(data.entries || [], currentLeafId);
  renderMessages(data);
})();
`

type ExportHTMLSessionData struct {
	Header        *SessionHeader                    `json:"header,omitempty"`
	Entries       []FileEntry                       `json:"entries"`
	LeafID        *string                           `json:"leafId"`
	SystemPrompt  string                            `json:"systemPrompt,omitempty"`
	Tools         []ExportHTMLToolDefinition        `json:"tools,omitempty"`
	RenderedTools map[string]ExportHTMLRenderedTool `json:"renderedTools,omitempty"`
}

type ExportHTMLToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type ExportHTMLRenderedTool struct {
	CallHTML            string `json:"callHtml,omitempty"`
	ResultHTMLCollapsed string `json:"resultHtmlCollapsed,omitempty"`
	ResultHTMLExpanded  string `json:"resultHtmlExpanded,omitempty"`
}

func AnsiLinesToHTML(lines []string) string {
	var builder strings.Builder
	for _, line := range lines {
		builder.WriteString(`<div class="ansi-line">`)
		builder.WriteString(ansiLineToHTML(line))
		builder.WriteString(`</div>`)
	}
	return builder.String()
}

func RenderCustomToolResultHTML(lines []string) string {
	start := 0
	for start < len(lines) && isBlankRenderedHTMLLine(lines[start]) {
		start++
	}
	end := len(lines)
	for end > start && isBlankRenderedHTMLLine(lines[end-1]) {
		end--
	}
	return AnsiLinesToHTML(lines[start:end])
}

func DefaultSessionExportHTMLName(inputPath string) string {
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	if strings.TrimSpace(base) == "" {
		base = "session"
	}
	return "gi-session-" + base + ".html"
}

func ExportSessionFileToHTML(inputPath, outputPath string) (string, error) {
	return ExportSessionFileToHTMLWithOptions(inputPath, outputPath, ExportHTMLOptions{})
}

func ExportSessionFileToHTMLWithOptions(inputPath, outputPath string, options ExportHTMLOptions) (string, error) {
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" {
		return "", errors.New("session file path is required")
	}
	if _, err := os.Stat(inputPath); err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("File not found: " + inputPath)
		}
		return "", err
	}
	sessionManager, err := OpenSessionManager(inputPath)
	if err != nil {
		return "", err
	}
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		outputPath = DefaultSessionExportHTMLName(inputPath)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(outputPath, []byte(RenderSessionManagerHTMLWithOptions(sessionManager, options)), 0o644); err != nil {
		return "", err
	}
	return outputPath, nil
}

func RenderSessionManagerHTML(sessionManager *SessionManager) string {
	return RenderSessionManagerHTMLWithOptions(sessionManager, ExportHTMLOptions{})
}

func RenderSessionManagerHTMLWithOptions(sessionManager *SessionManager, options ExportHTMLOptions) string {
	data := BuildExportHTMLSessionDataWithOptions(sessionManager, options)
	sessionData := exportHTMLSessionDataBase64(data)
	var builder strings.Builder
	builder.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Gi Session Export</title>
  <style>
`)
	builder.WriteString(ExportHTMLTemplateCSSWithOptions(options))
	builder.WriteString(`  </style>
</head>
<body>
  <button id="hamburger" title="Open sidebar">tree</button>
  <div id="sidebar-overlay"></div>
  <div id="app">
    <aside id="sidebar">
      <div class="sidebar-header">
        <div class="sidebar-controls">
          <input type="text" class="sidebar-search" id="tree-search" placeholder="Search..." aria-label="Search session tree">
        </div>
        <div class="sidebar-filters">
          <button class="filter-btn active" data-filter="default" title="Hide settings entries">Default</button>
          <button class="filter-btn" data-filter="no-tools" title="Default minus tool results">No-tools</button>
          <button class="filter-btn" data-filter="user-only" title="Only user messages">User</button>
          <button class="filter-btn" data-filter="labeled-only" title="Only labeled entries">Labeled</button>
          <button class="filter-btn" data-filter="all" title="Show everything">All</button>
          <button class="sidebar-close" id="sidebar-close" title="Close sidebar">x</button>
        </div>
      </div>
      <div class="tree-container" id="tree-container"></div>
      <div class="tree-status" id="tree-status"></div>
    </aside>
    <div id="sidebar-resizer" role="separator" aria-orientation="vertical" aria-label="Resize session tree sidebar"></div>
    <main id="content">
      <div id="header-container"></div>
      <div id="messages"></div>
    </main>
  </div>
  <script id="session-data" type="application/json">`)
	builder.WriteString(html.EscapeString(sessionData))
	builder.WriteString(`</script>
  <script>
`)
	builder.WriteString(ExportHTMLTemplateJS)
	builder.WriteString(`  </script>
</body>
</html>
`)
	return builder.String()
}

func BuildExportHTMLSessionData(sessionManager *SessionManager) ExportHTMLSessionData {
	return BuildExportHTMLSessionDataWithOptions(sessionManager, ExportHTMLOptions{})
}

func BuildExportHTMLSessionDataWithOptions(sessionManager *SessionManager, options ExportHTMLOptions) ExportHTMLSessionData {
	if sessionManager == nil {
		return ExportHTMLSessionData{Entries: []FileEntry{}}
	}
	entries := sessionManager.GetEntries()
	renderedTools := PreRenderExportHTMLCustomTools(entries, options.ToolRenderer)
	return ExportHTMLSessionData{
		Header:        sessionManager.GetHeader(),
		Entries:       entries,
		LeafID:        sessionManager.GetLeafID(),
		RenderedTools: renderedTools,
	}
}

func exportHTMLSessionDataBase64(data ExportHTMLSessionData) string {
	payload, err := json.Marshal(data)
	if err != nil {
		payload = []byte(`{"entries":[]}`)
	}
	return base64.StdEncoding.EncodeToString(payload)
}

var exportHTMLTemplateRenderedTools = map[string]bool{
	"bash":  true,
	"read":  true,
	"write": true,
	"edit":  true,
	"ls":    true,
}

func PreRenderExportHTMLCustomTools(entries []FileEntry, renderer ExportHTMLToolRenderer) map[string]ExportHTMLRenderedTool {
	if renderer == nil || len(entries) == 0 {
		return nil
	}
	renderedTools := map[string]ExportHTMLRenderedTool{}
	for _, entry := range entries {
		if entry.Type != "message" {
			continue
		}
		message, ok := sessionMessageToLLM(entry.Message)
		if !ok {
			continue
		}
		switch message.Role {
		case llm.RoleAssistant:
			for _, part := range message.Content {
				if part.Type != llm.ContentToolCall || strings.TrimSpace(part.ID) == "" || exportHTMLTemplateRenderedTools[part.Name] {
					continue
				}
				callHTML := renderer.RenderCall(part.ID, part.Name, part.Arguments)
				if strings.TrimSpace(callHTML) == "" {
					continue
				}
				rendered := renderedTools[part.ID]
				rendered.CallHTML = callHTML
				renderedTools[part.ID] = rendered
			}
		case llm.RoleToolResult:
			toolCallID := strings.TrimSpace(message.ToolCallID)
			if toolCallID == "" || exportHTMLTemplateRenderedTools[message.ToolName] {
				continue
			}
			existing := renderedTools[toolCallID]
			result := renderer.RenderResult(toolCallID, message.ToolName, exportHTMLToolResultFromMessage(message), message.IsError)
			if strings.TrimSpace(result.Collapsed) == "" && strings.TrimSpace(result.Expanded) == "" {
				continue
			}
			existing.ResultHTMLCollapsed = result.Collapsed
			existing.ResultHTMLExpanded = result.Expanded
			renderedTools[toolCallID] = existing
		}
	}
	if len(renderedTools) == 0 {
		return nil
	}
	return renderedTools
}

func exportHTMLToolResultFromMessage(message llm.Message) FileToolResult {
	result := fileToolResultFromLLMMessage(message)
	if details, ok := message.Details.(*FileToolDetails); ok {
		result.Details = details
	}
	return result
}

type ExportHTMLSkillBlock struct {
	Name        string
	Location    string
	Content     string
	UserMessage string
}

type ExportHTMLSidebarEntry struct {
	Role  string
	Label string
}

var exportHTMLSkillBlockPattern = regexp.MustCompile(`(?s)^<skill\s+([^>]*)>\s*\n?(.*?)\n?</skill>\s*(.*)$`)
var exportHTMLSkillAttrPattern = regexp.MustCompile(`([A-Za-z_:][A-Za-z0-9_.:-]*)="([^"]*)"`)

func ParseExportHTMLSkillBlock(text string) (ExportHTMLSkillBlock, bool) {
	match := exportHTMLSkillBlockPattern.FindStringSubmatch(text)
	if match == nil {
		return ExportHTMLSkillBlock{}, false
	}
	attrs := parseExportHTMLSkillAttrs(match[1])
	name := attrs["name"]
	if name == "" {
		return ExportHTMLSkillBlock{}, false
	}
	return ExportHTMLSkillBlock{
		Name:        name,
		Location:    attrs["location"],
		Content:     strings.TrimSpace(match[2]),
		UserMessage: strings.TrimSpace(match[3]),
	}, true
}

func RenderExportHTMLUserMessage(text string) string {
	skillBlock, ok := ParseExportHTMLSkillBlock(text)
	if !ok {
		return `<div class="user-message">` + html.EscapeString(text) + `</div>`
	}
	var builder strings.Builder
	builder.WriteString(`<div class="skill-invocation"><div class="skill-name">`)
	builder.WriteString(html.EscapeString(skillBlock.Name))
	builder.WriteString(`</div><div class="skill-content">`)
	builder.WriteString(renderExportHTMLMarkdown(skillBlock.Content))
	builder.WriteString(`</div></div>`)
	if skillBlock.UserMessage != "" {
		builder.WriteString(`<div class="user-message">`)
		builder.WriteString(html.EscapeString(skillBlock.UserMessage))
		builder.WriteString(`</div>`)
	}
	return builder.String()
}

func RenderExportHTMLMarkdownLink(href, text string) string {
	if !isSafeExportHTMLURL(href) {
		return html.EscapeString(text)
	}
	return `<a href="` + html.EscapeString(href) + `">` + html.EscapeString(text) + `</a>`
}

func RenderExportHTMLMarkdownImage(src, alt string) string {
	if !isSafeExportHTMLURL(src) {
		return ""
	}
	return `<img src="` + html.EscapeString(src) + `" alt="` + html.EscapeString(alt) + `">`
}

func RenderExportHTMLInlineImage(mimeType, data string) string {
	return `<img src="data:` + html.EscapeString(mimeType) + `;base64,` + html.EscapeString(data) + `">`
}

func RenderExportHTMLSessionEntryAttrs(entryID string) string {
	escaped := html.EscapeString(entryID)
	return `id="entry-` + escaped + `" data-entry-id="` + escaped + `"`
}

func RenderExportHTMLTreeMetadata(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if values[key] == "" {
			continue
		}
		parts = append(parts, "["+html.EscapeString(key)+": "+html.EscapeString(values[key])+"]")
	}
	return strings.Join(parts, " ")
}

func RenderExportHTMLHeaderModels(models []string) string {
	if len(models) == 0 {
		return "unknown"
	}
	return html.EscapeString(strings.Join(models, ", "))
}

func ExportHTMLSidebarEntriesForUserMessage(text string) []ExportHTMLSidebarEntry {
	skillBlock, ok := ParseExportHTMLSkillBlock(text)
	if !ok {
		return []ExportHTMLSidebarEntry{{Role: "tree-role-user", Label: text}}
	}
	entries := []ExportHTMLSidebarEntry{{Role: "tree-role-skill", Label: skillBlock.Name}}
	if skillBlock.UserMessage != "" {
		entries = append(entries, ExportHTMLSidebarEntry{Role: "tree-role-user", Label: skillBlock.UserMessage})
	}
	return entries
}

func isSafeExportHTMLURL(rawURL string) bool {
	normalized := strings.ToLower(strings.TrimSpace(rawURL))
	normalized = strings.ReplaceAll(normalized, "\u0000", "")
	return !strings.HasPrefix(normalized, "javascript:") && !strings.HasPrefix(normalized, "vbscript:")
}

func parseExportHTMLSkillAttrs(raw string) map[string]string {
	attrs := map[string]string{}
	for _, match := range exportHTMLSkillAttrPattern.FindAllStringSubmatch(raw, -1) {
		attrs[match[1]] = html.UnescapeString(match[2])
	}
	return attrs
}

func renderExportHTMLMarkdown(markdown string) string {
	escaped := html.EscapeString(markdown)
	boldPattern := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	return boldPattern.ReplaceAllString(escaped, `<strong>$1</strong>`)
}

var exportHTMLANSIRegex = regexp.MustCompile(`\x1b\[([\d;]*)m`)

var exportHTMLANSIColors = []string{
	"#000000",
	"#800000",
	"#008000",
	"#808000",
	"#000080",
	"#800080",
	"#008080",
	"#c0c0c0",
	"#808080",
	"#ff0000",
	"#00ff00",
	"#ffff00",
	"#0000ff",
	"#ff00ff",
	"#00ffff",
	"#ffffff",
}

type exportHTMLTextStyle struct {
	fg        string
	bg        string
	bold      bool
	dim       bool
	italic    bool
	underline bool
}

func isBlankRenderedHTMLLine(line string) bool {
	return strings.TrimSpace(exportHTMLANSIRegex.ReplaceAllString(line, "")) == ""
}

func ansiLineToHTML(line string) string {
	style := exportHTMLTextStyle{}
	var builder strings.Builder
	lastIndex := 0
	inSpan := false
	matches := exportHTMLANSIRegex.FindAllStringSubmatchIndex(line, -1)
	for _, match := range matches {
		if match[0] > lastIndex {
			builder.WriteString(escapeExportHTMLText(line[lastIndex:match[0]]))
		}
		if inSpan {
			builder.WriteString(`</span>`)
			inSpan = false
		}
		paramText := ""
		if match[2] >= 0 && match[3] >= 0 {
			paramText = line[match[2]:match[3]]
		}
		applyExportHTMLSGRCode(parseExportHTMLSGRParams(paramText), &style)
		if exportHTMLStyleHasValue(style) {
			builder.WriteString(`<span style="`)
			builder.WriteString(exportHTMLStyleToInlineCSS(style))
			builder.WriteString(`">`)
			inSpan = true
		}
		lastIndex = match[1]
	}
	if lastIndex < len(line) {
		builder.WriteString(escapeExportHTMLText(line[lastIndex:]))
	}
	if inSpan {
		builder.WriteString(`</span>`)
	}
	if builder.Len() == 0 {
		return "&nbsp;"
	}
	return builder.String()
}

func parseExportHTMLSGRParams(raw string) []int {
	if raw == "" {
		return []int{0}
	}
	parts := strings.Split(raw, ";")
	params := make([]int, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			value = 0
		}
		params = append(params, value)
	}
	return params
}

func applyExportHTMLSGRCode(params []int, style *exportHTMLTextStyle) {
	for i := 0; i < len(params); i++ {
		code := params[i]
		switch {
		case code == 0:
			*style = exportHTMLTextStyle{}
		case code == 1:
			style.bold = true
		case code == 2:
			style.dim = true
		case code == 3:
			style.italic = true
		case code == 4:
			style.underline = true
		case code == 22:
			style.bold = false
			style.dim = false
		case code == 23:
			style.italic = false
		case code == 24:
			style.underline = false
		case code >= 30 && code <= 37:
			style.fg = exportHTMLANSIColors[code-30]
		case code == 38:
			if i+2 < len(params) && params[i+1] == 5 {
				style.fg = exportHTMLColor256ToHex(params[i+2])
				i += 2
			} else if i+4 < len(params) && params[i+1] == 2 {
				style.fg = "rgb(" + strconv.Itoa(params[i+2]) + "," + strconv.Itoa(params[i+3]) + "," + strconv.Itoa(params[i+4]) + ")"
				i += 4
			}
		case code == 39:
			style.fg = ""
		case code >= 40 && code <= 47:
			style.bg = exportHTMLANSIColors[code-40]
		case code == 48:
			if i+2 < len(params) && params[i+1] == 5 {
				style.bg = exportHTMLColor256ToHex(params[i+2])
				i += 2
			} else if i+4 < len(params) && params[i+1] == 2 {
				style.bg = "rgb(" + strconv.Itoa(params[i+2]) + "," + strconv.Itoa(params[i+3]) + "," + strconv.Itoa(params[i+4]) + ")"
				i += 4
			}
		case code == 49:
			style.bg = ""
		case code >= 90 && code <= 97:
			style.fg = exportHTMLANSIColors[code-90+8]
		case code >= 100 && code <= 107:
			style.bg = exportHTMLANSIColors[code-100+8]
		}
	}
}

func exportHTMLStyleHasValue(style exportHTMLTextStyle) bool {
	return style.fg != "" || style.bg != "" || style.bold || style.dim || style.italic || style.underline
}

func exportHTMLStyleToInlineCSS(style exportHTMLTextStyle) string {
	parts := []string{}
	if style.fg != "" {
		parts = append(parts, "color:"+style.fg)
	}
	if style.bg != "" {
		parts = append(parts, "background-color:"+style.bg)
	}
	if style.bold {
		parts = append(parts, "font-weight:bold")
	}
	if style.dim {
		parts = append(parts, "opacity:0.6")
	}
	if style.italic {
		parts = append(parts, "font-style:italic")
	}
	if style.underline {
		parts = append(parts, "text-decoration:underline")
	}
	return strings.Join(parts, ";")
}

func exportHTMLColor256ToHex(index int) string {
	if index >= 0 && index < 16 {
		return exportHTMLANSIColors[index]
	}
	if index >= 16 && index < 232 {
		cubeIndex := index - 16
		r := cubeIndex / 36
		g := (cubeIndex % 36) / 6
		b := cubeIndex % 6
		return "#" + exportHTMLColorCubeComponent(r) + exportHTMLColorCubeComponent(g) + exportHTMLColorCubeComponent(b)
	}
	if index < 232 {
		index = 232
	}
	if index > 255 {
		index = 255
	}
	gray := 8 + (index-232)*10
	return "#" + exportHTMLHexByte(gray) + exportHTMLHexByte(gray) + exportHTMLHexByte(gray)
}

func exportHTMLColorCubeComponent(value int) string {
	if value == 0 {
		return "00"
	}
	return exportHTMLHexByte(55 + value*40)
}

func exportHTMLHexByte(value int) string {
	if value < 0 {
		value = 0
	}
	if value > 255 {
		value = 255
	}
	hexValue := strings.ToLower(strconv.FormatInt(int64(value), 16))
	if len(hexValue) < 2 {
		return "0" + hexValue
	}
	return hexValue
}

func escapeExportHTMLText(text string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#039;",
	).Replace(text)
}
