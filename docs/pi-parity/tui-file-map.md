# TUI File Map

This document tracks Pi `packages/tui/src` against Gi `gi-tui`. It is a working
file/function parity audit, not a completion claim.

## Status Legend

- `direct`: Gi has a same-purpose file/function implementation.
- `split`: Pi's file is represented by multiple Go files.
- `consolidated`: Pi's small file is folded into a larger Go file.
- `go-native`: Pi's Node/TS-specific surface is represented by Go-native code.
- `partial`: Gi has some equivalent behavior, but the file/function audit is not
  complete or a Pi feature is missing.
- `gap`: no current Gi equivalent.

## Root Source Files

| Pi file | Pi exported surface / major functions | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `autocomplete.ts` | `AutocompleteItem`, `SlashCommand`, `AutocompleteSuggestions`, `AutocompleteProvider`, `CombinedAutocompleteProvider` | `autocomplete.go` | direct | Command/path/custom provider composition is represented, including context-aware suggestions and mutable command/base-path state. |
| `editor-component.ts` | `EditorComponent` custom-editor interface | `tui.go`, `components_editor.go` `Editor` | direct | Gi exposes `EditorComponent` and `Focusable` contracts through Go interfaces. |
| `fuzzy.ts` | `FuzzyMatch`, `fuzzyMatch`, `fuzzyFilter` | `autocomplete.go` `FuzzyMatchText`, `FuzzyFilter` | direct | Fuzzy scoring/filtering helpers are represented. |
| `index.ts` | public package re-exports | Go package exports plus `api_parity_test.go` | consolidated | Go does not need a barrel file; `TestPiTUIIndexPublicSurfaceCompiles` guards the public surface. |
| `keybindings.ts` | `TUI_KEYBINDINGS`, `KeybindingsManager`, conflict/default resolution | `keybindings.go` | direct | Defaults, user bindings, conflicts, resolved bindings, matching, and global manager state are represented, including the v0.82 `ctrl+j` newline alias. |
| `keys.ts` | `Key`, `matchesKey`, `parseKey`, Kitty/modifyOtherKeys helpers, release/repeat detection | `keys.go` | direct | Gi covers Kitty protocol state, CSI-u/modifyOtherKeys parsing, key IDs, repeat/release detection, and printable decoding. |
| `kill-ring.ts` | `KillRing` | `history.go` | direct | Push/peek/rotate/length semantics are represented. |
| `native-modifiers.ts` | native modifier state used by Apple Terminal input normalization | `native_modifiers.go`, `native_modifiers_darwin.go`, `native_modifiers_unsupported.go` | go-native | Gi uses build-selected implementations: ApplicationServices on Darwin with cgo and a deterministic unsupported-platform fallback. |
| `stdin-buffer.ts` | `StdinBuffer`, paste/sequence splitting, incomplete escape buffering | `stdin_buffer.go` | direct | Sequence completeness, paste handling, Kitty/mouse dedupe, timeout flush, and callback locking behavior are covered by Go tests. |
| `terminal-colors.ts` | OSC 11 background colors and CSI color-scheme reports | `terminal_colors.go` | direct | Strict response recognition, variable-width hexadecimal channel scaling, FIFO queries, late-response consumption, and color-scheme notifications are represented. |
| `terminal-image.ts` | image protocol detection, Kitty/iTerm2 encoding, dimensions, fallbacks, hyperlinks | `terminal_image.go` | direct | Capabilities, image IDs, encoders, dimension readers, fallback text, hyperlink helpers, and bounded tmux hyperlink probing are represented. |
| `terminal.ts` | `Terminal`, `ProcessTerminal` raw mode, resize, cursor/title/progress/Kitty keyboard | `terminal.go`, `terminal_native.go`, `terminal_resize_*.go`, `native_modifiers*.go` | direct | Go implementation is native rather than Node stream-based; raw mode, start/stop, resize, cursor, title, progress, input drain, DA-sentinel keyboard negotiation, split-response buffering, modifyOtherKeys fallback, and Apple Terminal Shift+Enter normalization are represented. |
| `tui.ts` | `Component`, `Focusable`, `Container`, `TUI`, overlays, diff rendering, cursor marker | `tui.go`, `virtual_terminal.go` | direct | Container mutation, overlay focus restoration, differential/full redraw, multi-row Kitty placement cleanup, cursor marker, and clear-on-shrink policies are represented. |
| `undo-stack.ts` | `UndoStack` | `history.go` | direct | Snapshot push/pop/length semantics are represented. |
| `utils.ts` | `visibleWidth`, `wrapTextWithAnsi`, `truncateToWidth`, ANSI slicing/segments, background application | `utils.go`, `ansi_shared.go`, `internal/width`, `internal/vtemu/width.go` | direct | Visual-width and ANSI-aware wrapping/truncation/slicing helpers are represented; shared width logic excludes graphemes crossing overlay boundaries and expands only visible tabs while preserving control-string bytes. |
| `word-navigation.ts` | pure forward/backward word movement with custom and atomic segmentation | `word_navigation.go`, `components_editor.go`, `components_input.go` | direct | Gi measures cursor positions in runes, exposes a custom `WordSegmenter`, and shares the same pure helpers between `Input` and `Editor`; valid paste markers remain indivisible. |

## Component Files

| Pi file | Pi exported surface / major functions | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `components/box.ts` | `Box` padded background container | `components_basic.go` `Box` | direct | Child ordering, padding, background, clear/remove, and width clipping are represented. |
| `components/cancellable-loader.ts` | `CancellableLoader` with abort handling | `components_loader.go` `CancellableLoader` | direct | Cancel/abort/context/dispose behavior and Pi-style `OnAbort` callback are represented. |
| `components/editor.ts` | `Editor`, editor options/theme, wrapping, autocomplete, history, key handling, cursor marker | `components_editor.go` `Editor`, `history.go`, `autocomplete.go` | direct | Editor rendering/input/autocomplete/history behavior is consolidated in Go, including provider-defined trigger characters, draft-preserving history traversal, and narrow scroll borders. |
| `components/image.ts` | `Image`, image options/theme/fallback | `components_image.go` `Image`, `terminal_image.go` | direct | Terminal image render/fallback and image ID helpers are represented. |
| `components/input.ts` | `Input` focusable single-line input | `components_input.go` `Input` | direct | Value, focus, cursor, placeholder, and submit/change behavior are represented. |
| `components/loader.ts` | `Loader`, spinner frames/interval/colors/verbatim rendering | `components_loader.go` `Loader` | direct | Message updates, indicator customization, TUI-driven rerendering, stop/start, and color hooks are represented. |
| `components/markdown.ts` | `Markdown`, `MarkdownTheme`, default text style, links/code/lists/tables/html/blockquote | `components_markdown.go`, `markdown_goldmark.go` | partial | Public surface and many Pi fixture behaviors are represented. Implementation differs: Pi uses `marked`; Gi parses with goldmark plus custom renderer compatibility logic, so fixture-level parity remains an ongoing risk. |
| `components/select-list.ts` | `SelectList`, theme, primary-column truncation, filtering, layout | `components_select_list.go` `SelectList` | direct | Filtering, selected item, layout options, scroll info, and truncation callback are represented. |
| `components/settings-list.ts` | `SettingsList`, setting items, search, submenu/change/cancel callbacks | `components_settings_list.go` `SettingsList` | direct | Search, value updates, submenu construction, callbacks, and theme hooks are represented. |
| `components/spacer.ts` | `Spacer` | `components_basic.go` `Spacer` | direct | Line-count spacing and mutation are represented. |
| `components/text.ts` | `Text` padded styled text | `components_basic.go` `Text` | direct | Padding, text mutation, and custom background are represented. |
| `components/truncated-text.ts` | `TruncatedText` | `components_basic.go` `TruncatedText` | direct | Width-aware truncation with configurable ellipsis/style is represented. |

## Function-Level Checkpoints Completed In This Pass

- Pi `index.ts` public exports are guarded by Gi
  `gi-tui/api_parity_test.go` `TestPiTUIIndexPublicSurfaceCompiles`.
- Pi `Loader`/`CancellableLoader` map to Gi `Loader`/`CancellableLoader`,
  including indicator customization and abort callback semantics.
- Pi `TUI` overlay options map to Gi `OverlayOptions`, including percent/decimal
  sizes, margins, max height, non-capturing behavior, focus restoration, and
  visible predicates.
- Pi `ProcessTerminal` progress/title/cursor/keyboard negotiation maps to Gi
  `ProcessTerminal` tests, including progress keepalive cleanup and Kitty
  keyboard fallback.
- Pi `StdinBuffer` escape-sequence and paste handling maps to Gi
  `stdin_buffer.go` and is covered by the Pi matrix tests in `terminal_test.go`.
- Pi v0.82 terminal input negotiation maps to a single Go-owned state machine:
  Kitty flags and DA responses are consumed, incomplete negotiation prefixes
  are buffered and replayed, and modifyOtherKeys is enabled only after a
  negative Kitty result.
- Terminal color replies are consumed before general input listeners and
  focused components. OSC 11 requests retain FIFO reply slots after context
  timeout so late terminal responses cannot leak into editor input.

## Pi v0.82 Terminal Input And Color Symbols

| Pi file | Pi implementation symbols | Gi equivalent | Status |
| --- | --- | --- | --- |
| `native-modifiers.ts` | `isNativeModifiersHelper`, `loadNativeModifiersHelper`, `isNativeModifierPressed` | build-selected `nativeModifierPressed`, public `IsNativeModifierPressed` | go-native |
| `terminal-colors.ts` | `hexToRgb`, `parseOscHexChannel`, `isOsc11BackgroundColorResponse`, `parseOsc11BackgroundColor`, `parseTerminalColorSchemeReport` | `hexToRGB`, `parseOSCHexChannel`, `IsOSC11BackgroundColorResponse`, `ParseOSC11BackgroundColor`, `ParseTerminalColorSchemeReport` | direct |
| `terminal.ts` | `parseKeyboardProtocolNegotiationSequence`, `isKeyboardProtocolNegotiationSequencePrefix`, `isAppleTerminalSession`, `normalizeAppleTerminalInput` | `ParseKeyboardProtocolNegotiationSequence`, `isKeyboardProtocolNegotiationSequencePrefix`, `IsAppleTerminalSession`, `NormalizeAppleTerminalInput` | direct |
| `terminal.ts` | `ProcessTerminal.modifyOtherKeysActive`, `ProcessTerminal.handleKeyboardProtocolNegotiationSequence`, `ProcessTerminal.readKeyboardProtocolNegotiationSequence`, `ProcessTerminal.setKeyboardProtocolNegotiationBuffer`, `ProcessTerminal.clearKeyboardProtocolNegotiationBuffer` | `ProcessTerminal.ModifyOtherKeysActive`, `handleKeyboardProtocolNegotiationSequence`, `readKeyboardProtocolNegotiationSequence`, `setKeyboardProtocolNegotiationBufferLocked`, `clearKeyboardProtocolNegotiationBufferLocked` | direct |
| `terminal.ts` | `ProcessTerminal.flushKeyboardProtocolNegotiationBufferAsInput`, `ProcessTerminal.scheduleKeyboardProtocolNegotiationBufferFlush`, `ProcessTerminal.clearKeyboardProtocolNegotiationBufferFlushTimer`, `ProcessTerminal.forwardInputSequence`, `ProcessTerminal.enableModifyOtherKeys`, `ProcessTerminal.disableModifyOtherKeys` | same-purpose Go methods in `terminal.go`, with `Locked` suffixes where the lock contract is explicit | direct |
| `tui.ts` | `TUI.onTerminalColorSchemeChange`, `TUI.setTerminalColorSchemeNotifications`, `TUI.consumeOsc11BackgroundResponse`, `TUI.consumeTerminalColorSchemeReport`, `TUI.queryTerminalBackgroundColor`, `TUI.queryTerminalColorScheme` | `OnTerminalColorSchemeChange`, `SetTerminalColorSchemeNotifications`, `consumeOSC11BackgroundResponse`, `consumeTerminalColorSchemeReport`, `QueryTerminalBackgroundColor`, `QueryTerminalColorScheme` | direct |
| `utils.ts` | `getGraphemeSegmenter`, `getWordSegmenter` | shared grapheme spans in `internal/width`; `defaultWordSegments` plus injectable `WordSegmenter` in `word_navigation.go` | go-native |
| `word-navigation.ts` | `findWordBackward`, `findWordForward` | `FindWordBackward`, `FindWordForward` | direct |

## Pi v0.82 Editor, Markdown, Image, And TUI Symbols

| Pi file | Pi implementation symbols | Gi equivalent | Status |
| --- | --- | --- | --- |
| `components/editor.ts` | `escapeCharacterClass`, `buildTriggerPattern`, `buildDebouncePattern`, `createScrollBorder` | same-named Go helpers in `components_editor.go` | direct |
| `components/editor.ts` | `Editor.exitHistoryBrowsing`, `Editor.setAutocompleteTriggerCharacters` | same-named Go methods in `components_editor.go` | direct |
| `components/markdown.ts` | `trimPartialClosingFences`, `Markdown.getOrderedListMarker`, `Markdown.getUnorderedListMarker` | same-named Go helpers/methods in `components_markdown.go` | direct |
| `terminal-image.ts` | `probeTmuxHyperlinks` | same-named bounded Go probe in `terminal_image.go` | go-native |
| `tui.ts` | `parseKittyImageHeader`, `extractKittyImageRows` | same-named Go parsing helpers in `tui.go` | direct |
| `tui.ts` | `TUI.setFocusInternal`, `TUI.clearOverlayFocusRestore`, `TUI.clearOverlayFocusRestoreFor`, `TUI.resolveBlockedOverlayFocusResume`, `TUI.getVisibleOverlayFocusRestore`, `TUI.isOverlayFocusAncestor`, `TUI.retargetOverlayPreFocus`, `TUI.isComponentMounted`, `TUI.containsComponent` | same-named Go methods plus typed focus-state enums in `tui.go` | direct |
| `tui.ts` | `TUI.getKittyImageReservedRows`, `TUI.expandChangedRangeForKittyImages` | same-named Go methods in `tui.go` | direct |

## Exported Symbol Audit

This section pins Pi exported symbols that are not obvious from the file-level
rows above. It is the template for the remaining modules: every exported Pi
symbol should either map to a Gi public symbol, be folded into an explicitly
named Go implementation, or be called out as a gap.

### Component Option And Theme Types

| Pi symbol | Gi equivalent | Status | Notes |
| --- | --- | --- | --- |
| `EditorOptions` | `components_editor.go` `EditorOptions` | direct | Editor construction options are public in Gi. |
| `EditorTheme` | `components_editor.go` `EditorTheme` | direct | Editor theme hooks are public in Gi. |
| `TextChunk` | `components_editor.go` `TextChunk` | direct | Word-wrap output chunks are public in Gi. |
| `wordWrapLine` | `components_editor.go` `WordWrapLine` | direct | Go exports this helper with Go-style capitalization. |
| `ImageOptions` | `components_image.go` `ImageOptions` | direct | Image component options are public in Gi. |
| `ImageTheme` | `components_image.go` `ImageTheme` | direct | Image fallback theme hooks are public in Gi. |
| `LoaderIndicatorOptions` | `components_loader.go` `LoaderIndicatorOptions` | direct | Loader indicator customization is public in Gi. |
| `DefaultTextStyle` | `components_markdown.go` `DefaultTextStyle` | direct | Markdown default text styling is public in Gi. |
| `SelectItem` | `components_select_list.go` `SelectItem` | direct | Select-list item contract is public in Gi. |
| `SelectListLayoutOptions` | `components_select_list.go` `SelectListLayoutOptions` | direct | Select-list layout options are public in Gi. |
| `SelectListTheme` | `components_select_list.go` `SelectListTheme` | direct | Select-list theme hooks are public in Gi. |
| `SelectListTruncatePrimaryContext` | `components_select_list.go` `SelectListTruncatePrimaryContext` | direct | Primary-column truncation callback context is public in Gi. |
| `SettingItem` | `components_settings_list.go` `SettingItem` | direct | Settings-list item contract is public in Gi. |
| `SettingsListOptions` | `components_settings_list.go` `SettingsListOptions` | direct | Settings-list construction options are public in Gi. |
| `SettingsListTheme` | `components_settings_list.go` `SettingsListTheme` | direct | Settings-list theme hooks are public in Gi. |

### Key And Keybinding Symbols

| Pi symbol | Gi equivalent | Status | Notes |
| --- | --- | --- | --- |
| `Keybinding`, `KeybindingConflict`, `KeybindingDefinition`, `KeybindingDefinitions` | `keybindings.go` same names | direct | Shape is Go structs/maps instead of TS interfaces. |
| `Keybindings`, `KeybindingsConfig`, `KeybindingsManager` | `keybindings.go` same names | direct | Manager methods expose resolved/user bindings and conflicts. |
| `getKeybindings`, `setKeybindings` | `GetKeybindings`, `SetKeybindings` | direct | Go-style capitalization. |
| `KeyEventType` | `keys.go` `KeyEventType` | direct | Press/repeat/release constants are public. |
| `KeyId` | `keys.go` `KeyID` | direct | Same concept with Go initialism capitalization. |
| `decodeKittyPrintable`, `decodePrintableKey` | `DecodeKittyPrintable`, `DecodePrintableKey` | direct | Go-style capitalization. |
| `isKeyRelease`, `isKeyRepeat` | `IsKeyRelease`, `IsKeyRepeat` | direct | Go-style capitalization. |
| `isKittyProtocolActive`, `setKittyProtocolActive` | `IsKittyProtocolActive`, `SetKittyProtocolActive` | direct | Global Kitty keyboard protocol state is public in Gi. |

### Stdin And Terminal Image Symbols

| Pi symbol | Gi equivalent | Status | Notes |
| --- | --- | --- | --- |
| `StdinBufferEventMap` | `stdin_buffer.go` `StdinBufferEventMap` | direct | Callback map is public in Gi. |
| `StdinBufferOptions` | `stdin_buffer.go` `StdinBufferOptions` | direct | Paste timeout and callbacks are public in Gi. |
| `CellDimensions`, `ImageCellSize`, `ImageDimensions` | `terminal_image.go` same names | direct | Dimension contracts are public in Gi. |
| `ImageProtocol`, `ImageRenderOptions`, `TerminalCapabilities` | `terminal_image.go` same names | direct | Terminal image protocol/capability contracts are public in Gi. |
| `allocateImageId` | `AllocateImageID` | direct | Go-style capitalization and ID initialism. |
| `calculateImageCellSize`, `calculateImageRows` | `CalculateImageCellSize`, `CalculateImageRows` | direct | Cell-size calculations are public in Gi. |
| `deleteAllKittyImages`, `deleteKittyImage` | `DeleteAllKittyImages`, `DeleteKittyImage` | direct | Kitty cleanup sequences are public in Gi. |
| `detectCapabilities`, `getCapabilities`, `setCapabilities`, `resetCapabilitiesCache` | `DetectCapabilities`, `GetCapabilities`, `SetCapabilities`, `ResetCapabilitiesCache` | direct | Capability detection/cache lifecycle is public in Gi. |
| `encodeITerm2`, `encodeKitty`, `renderImage` | `EncodeITerm2`, `EncodeKitty`, `RenderImage` | direct | Go-style capitalization. |
| `getCellDimensions`, `setCellDimensions` | `GetCellDimensions`, `SetCellDimensions` | direct | Cell dimensions are public in Gi. |
| `getGifDimensions`, `getImageDimensions`, `getJpegDimensions`, `getPngDimensions`, `getWebpDimensions` | `GetGifDimensions`, `GetImageDimensions`, `GetJpegDimensions`, `GetPngDimensions`, `GetWebpDimensions` | direct | Image dimension helpers are public in Gi. |
| `imageFallback` | `ImageFallback`, `ImageFallbackDescription` | direct | Gi supports both compact alt fallback and descriptive fallback text. |
| `isImageLine` | `IsImageLine` | direct | Escape-sequence image-line detection is public in Gi. |
| `hyperlink` | `Hyperlink` | direct | OSC 8 hyperlink helper is public in Gi. |

### TUI Overlay And Utility Symbols

| Pi symbol | Gi equivalent | Status | Notes |
| --- | --- | --- | --- |
| `CURSOR_MARKER` | `tui.go` `CursorMarker` | direct | Go exposes the same cursor marker with Go-style naming. |
| `OverlayAnchor`, `OverlayHandle`, `OverlayMargin`, `SizeValue` | `tui.go` same names | direct | Overlay geometry/control contracts are public in Gi. |
| `isFocusable` | `IsFocusable` | direct | Go-style capitalization. |
| `applyBackgroundToLine` | `ApplyBackgroundToLine` | direct | Background fill helper is public in Gi. |
| `extractAnsiCode` | `ExtractAnsiCode` | direct | Gi also exposes `ANSICode` for structured return. |
| `extractSegments` | `ExtractSegments` | direct | Gi also exposes `ExtractedSegments` for structured return. |
| `normalizeTerminalOutput` | `NormalizeTerminalOutput` | direct | Thai/Lao terminal normalization is public in Gi. |
| `sliceByColumn`, `sliceWithWidth` | `SliceByColumn`, `SliceWithWidth` | direct | Gi also exposes `ColumnSlice` for width-aware slicing. |
| `isPunctuationChar`, `isWhitespaceChar` | `IsPunctuationChar`, `IsWhitespaceChar` | direct | Go-style capitalization. |
| `getSegmenter` | internal grapheme segmentation in `internal/width`, wrapped by `utils.go` | go-native | Go does not expose a JS `Intl.Segmenter` equivalent; width logic is implemented through shared grapheme span helpers and covered by visible-width tests. |

## Internal Top-Level Implementation Checkpoints

The exported-symbol audit above proves public ownership. This section tracks
Pi's non-exported top-level helper functions/classes in `packages/tui/src` so
the implementation audit does not silently skip private behavior.

| Pi file | Pi internal helpers | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `autocomplete.ts` | `toDisplayPath`, `escapeRegex`, `buildFdPathQuery`, `findLastDelimiter`, `findUnclosedQuoteStart`, `isTokenStart`, `extractQuotedPrefix`, `parsePathPrefix`, `buildCompletionValue`, `walkDirectoryWithFd` | `autocomplete.go` path-prefix parsing, quoted-prefix extraction, completion-value construction, directory walking | direct | Gi keeps the same autocomplete responsibilities but uses Go path/file APIs instead of `fd`. |
| `components/editor.ts` | `isPasteMarker`, `segmentWithMarkers` | `components_editor.go` editor paste-marker spans, `segmentLineForWrap`, `wordWrapLineWithSegments` | direct | Paste marker segmentation is represented inside the Go editor implementation. |
| `components/markdown.ts` | `StrictStrikethroughTokenizer` | `markdown_goldmark.go`, `components_markdown.go` strikethrough parsing/rendering | direct | Gi uses goldmark extension/parser hooks instead of Pi's marked tokenizer class. |
| `keybindings.ts` | `normalizeKeys` | `keybindings.go` keybinding normalization during manager/config construction | direct | Multi-key and single-key config normalization is represented in Go. |
| `keys.ts` | `normalizeKittyFunctionalCodepoint`, `normalizeShiftedLetterIdentityCodepoint`, `parseEventType`, `parseKittySequence`, `matchesKittySequence`, `parseModifyOtherKeysSequence`, `matchesModifyOtherKeys`, `isWindowsTerminalSession`, `matchesRawBackspace`, `rawCtrlChar`, `isDigitKey`, `matchesPrintableModifyOtherKeys`, `formatKeyNameWithModifiers`, `parseKeyId`, `formatParsedKey`, `decodeModifyOtherKeysPrintable` | `keys.go` Kitty/modifyOtherKeys parser and matcher helpers | direct | Helper names differ, but Kitty protocol state, CSI-u/modifyOtherKeys parsing, repeat/release events, raw control chars, and printable decoding are covered by `keys.go` tests. |
| `stdin-buffer.ts` | `isCompleteSequence`, `isCompleteCsiSequence`, `isCompleteOscSequence`, `isCompleteDcsSequence`, `isCompleteApcSequence`, `parseUnmodifiedKittyPrintableCodepoint`, `extractCompleteSequences` | `stdin_buffer.go` `extractCompleteInputSequences`, sequence completeness checks, Kitty printable handling | direct | Gi folds sequence-specific helpers into the input-sequence splitter and covers incomplete escape buffering in tests. |
| `tui.ts` | `extractKittyImageIds`, `parseSizeValue`, `isTermuxSession` | `tui.go` `extractKittyImageIDs`, overlay size parsing, terminal resize/platform handling | direct | Kitty image cleanup and overlay geometry behavior are represented; Termux-specific behavior is handled through Go terminal/platform checks. |
| `utils.ts` | `couldBeEmoji`, `isPrintableAscii`, `truncateFragmentToWidth`, `finalizeTruncatedResult`, `graphemeWidth`, `parseOsc8Hyperlink`, `formatOsc8Hyperlink`, `formatOsc8Close`, `AnsiCodeTracker`, `updateTrackerFromText`, `splitIntoTokensWithAnsi`, `wrapSingleLine`, `breakLongWord` | `utils.go`, `internal/width`, ANSI tracker, OSC 8 hyperlink tracking, wrapping/truncation helpers | direct | Gi implements these as Go helper functions/types around `VisibleWidth`, `WrapTextWithANSI`, `TruncateToWidth`, and ANSI slicing, with shared grapheme width rules factored into `internal/width`. |

## Internal Member-Level Checkpoints

These entries are tracked by
`docs/pi-parity/verify-source-map.mjs --scope members` so component methods,
terminal methods, and helper class methods are not skipped by the
file/top-level audit.

| Pi class | Pi member symbols | Gi equivalent | Status | Notes |
| --- | --- | --- | --- | --- |
| `CombinedAutocompleteProvider` | `CombinedAutocompleteProvider.constructor`, `CombinedAutocompleteProvider.getSuggestions`, `CombinedAutocompleteProvider.applyCompletion`, `CombinedAutocompleteProvider.extractAtPrefix`, `CombinedAutocompleteProvider.extractPathPrefix`, `CombinedAutocompleteProvider.expandHomePath`, `CombinedAutocompleteProvider.resolveScopedFuzzyQuery`, `CombinedAutocompleteProvider.scopedPathForDisplay`, `CombinedAutocompleteProvider.getFileSuggestions`, `CombinedAutocompleteProvider.scoreEntry`, `CombinedAutocompleteProvider.getFuzzyFileSuggestions`, `CombinedAutocompleteProvider.shouldTriggerFileCompletion` | `autocomplete.go` `CombinedAutocompleteProvider` methods and helpers | direct | Go keeps the same command/path/fuzzy autocomplete responsibilities with Go path walking. |
| `Box` | `Box.constructor`, `Box.addChild`, `Box.removeChild`, `Box.clear`, `Box.setBgFn`, `Box.invalidateCache`, `Box.matchCache`, `Box.invalidate`, `Box.render`, `Box.applyBg` | `components_basic.go` `Box` methods | direct | Child mutation, cache invalidation, background fill, and render behavior are represented. |
| `CancellableLoader` | `CancellableLoader.signal`, `CancellableLoader.aborted`, `CancellableLoader.handleInput`, `CancellableLoader.dispose` | `components_loader.go` `CancellableLoader` context/cancel methods | direct | Abort signal semantics are represented by Go context cancellation and disposal. |
| `Editor` | `Editor.constructor`, `Editor.validPasteIds`, `Editor.segment`, `Editor.getPaddingX`, `Editor.setPaddingX`, `Editor.getAutocompleteMaxVisible`, `Editor.setAutocompleteMaxVisible`, `Editor.setAutocompleteProvider`, `Editor.addToHistory`, `Editor.isEditorEmpty`, `Editor.isOnFirstVisualLine`, `Editor.isOnLastVisualLine`, `Editor.navigateHistory`, `Editor.setTextInternal`, `Editor.invalidate`, `Editor.render`, `Editor.handleInput`, `Editor.layoutText`, `Editor.getText`, `Editor.expandPasteMarkers`, `Editor.getExpandedText`, `Editor.getLines`, `Editor.getCursor`, `Editor.setText`, `Editor.insertTextAtCursor`, `Editor.normalizeText`, `Editor.insertTextAtCursorInternal`, `Editor.insertCharacter`, `Editor.handlePaste`, `Editor.addNewLine`, `Editor.shouldSubmitOnBackslashEnter`, `Editor.submitValue`, `Editor.handleBackspace`, `Editor.setCursorCol`, `Editor.moveToVisualLine`, `Editor.computeVerticalMoveColumn`, `Editor.moveToLineStart`, `Editor.moveToLineEnd`, `Editor.deleteToStartOfLine`, `Editor.deleteToEndOfLine`, `Editor.deleteWordBackwards`, `Editor.deleteWordForward`, `Editor.handleForwardDelete`, `Editor.buildVisualLineMap`, `Editor.findVisualLineAt`, `Editor.findCurrentVisualLine`, `Editor.moveCursor`, `Editor.pageScroll`, `Editor.moveWordBackwards`, `Editor.yank`, `Editor.yankPop`, `Editor.insertYankedText`, `Editor.deleteYankedText`, `Editor.pushUndoSnapshot`, `Editor.undo`, `Editor.jumpToChar`, `Editor.moveWordForwards`, `Editor.isSlashMenuAllowed`, `Editor.isAtStartOfMessage`, `Editor.isInSlashCommandContext`, `Editor.getBestAutocompleteMatchIndex`, `Editor.createAutocompleteList`, `Editor.tryTriggerAutocomplete`, `Editor.handleTabCompletion`, `Editor.handleSlashCommandCompletion`, `Editor.forceFileAutocomplete`, `Editor.requestAutocomplete`, `Editor.startAutocompleteRequest`, `Editor.getAutocompleteDebounceMs`, `Editor.runAutocompleteRequest`, `Editor.isAutocompleteRequestCurrent`, `Editor.applyAutocompleteSuggestions`, `Editor.cancelAutocompleteRequest`, `Editor.clearAutocompleteUi`, `Editor.cancelAutocomplete`, `Editor.isShowingAutocomplete`, `Editor.updateAutocomplete` | `components_editor.go` `Editor`, `history.go`, `autocomplete.go` | direct | Rendering, cursor movement, editing commands, paste markers, history, yanking, slash/autocomplete state, and async autocomplete are consolidated in Go. |
| `Image` | `Image.constructor`, `Image.getImageId`, `Image.invalidate`, `Image.render` | `components_image.go` `Image`, `terminal_image.go` | direct | Image ID, invalidation, terminal protocol render, and fallback rendering are represented. |
| `Input` | `Input.getValue`, `Input.setValue`, `Input.handleInput`, `Input.insertCharacter`, `Input.handleBackspace`, `Input.handleForwardDelete`, `Input.deleteToLineStart`, `Input.deleteToLineEnd`, `Input.deleteWordBackwards`, `Input.deleteWordForward`, `Input.yank`, `Input.yankPop`, `Input.pushUndo`, `Input.undo`, `Input.moveWordBackwards`, `Input.moveWordForwards`, `Input.handlePaste`, `Input.invalidate`, `Input.render` | `components_input.go` `Input`, `history.go` | direct | Single-line editing, undo/yank, word movement, paste, invalidation, and rendering are represented. |
| `Loader` | `Loader.constructor`, `Loader.render`, `Loader.start`, `Loader.stop`, `Loader.setMessage`, `Loader.setIndicator`, `Loader.restartAnimation`, `Loader.updateDisplay` | `components_loader.go` `Loader` methods | direct | Spinner lifecycle, message/indicator mutation, and rerender scheduling are represented. |
| `Markdown` | `StrictStrikethroughTokenizer.del`, `Markdown.constructor`, `Markdown.setText`, `Markdown.invalidate`, `Markdown.render`, `Markdown.applyDefaultStyle`, `Markdown.getDefaultStylePrefix`, `Markdown.getStylePrefix`, `Markdown.getDefaultInlineStyleContext`, `Markdown.renderToken`, `Markdown.renderInlineTokens`, `Markdown.renderList`, `Markdown.getLongestWordWidth`, `Markdown.wrapCellText`, `Markdown.renderTable` | `components_markdown.go`, `markdown_goldmark.go` | partial | Public render responsibilities are represented, but Gi uses goldmark plus compatibility rendering rather than Pi's marked tokenizer internals. |
| `SelectList` | `SelectList.constructor`, `SelectList.setFilter`, `SelectList.setSelectedIndex`, `SelectList.invalidate`, `SelectList.render`, `SelectList.handleInput`, `SelectList.renderItem`, `SelectList.getPrimaryColumnWidth`, `SelectList.getPrimaryColumnBounds`, `SelectList.truncatePrimary`, `SelectList.getDisplayValue`, `SelectList.notifySelectionChange`, `SelectList.getSelectedItem` | `components_select_list.go` `SelectList` methods | direct | Filtering, navigation, selection callbacks, primary-column truncation, and rendering are represented. |
| `SettingsList` | `SettingsList.constructor`, `SettingsList.updateValue`, `SettingsList.invalidate`, `SettingsList.render`, `SettingsList.renderMainList`, `SettingsList.handleInput`, `SettingsList.activateItem`, `SettingsList.closeSubmenu`, `SettingsList.applyFilter`, `SettingsList.addHintLine` | `components_settings_list.go` `SettingsList` methods | direct | Settings search/list/submenu/change/cancel behavior is represented. |
| `Spacer` | `Spacer.constructor`, `Spacer.setLines`, `Spacer.invalidate`, `Spacer.render` | `components_basic.go` `Spacer` methods | direct | Mutable line spacing is represented. |
| `Text` | `Text.constructor`, `Text.setText`, `Text.setCustomBgFn`, `Text.invalidate`, `Text.render` | `components_basic.go` `Text` methods | direct | Padded styled text and background hooks are represented. |
| `TruncatedText` | `TruncatedText.constructor`, `TruncatedText.invalidate`, `TruncatedText.render` | `components_basic.go` `TruncatedText` methods | direct | Width-aware truncation and rendering are represented. |
| `KeybindingsManager` | `KeybindingsManager.constructor`, `KeybindingsManager.rebuild`, `KeybindingsManager.matches`, `KeybindingsManager.getKeys`, `KeybindingsManager.getDefinition`, `KeybindingsManager.getConflicts`, `KeybindingsManager.setUserBindings`, `KeybindingsManager.getUserBindings`, `KeybindingsManager.getResolvedBindings` | `keybindings.go` `KeybindingsManager` methods | direct | Default/user binding resolution, matching, and conflict reporting are represented. |
| `KillRing` | `KillRing.push`, `KillRing.peek`, `KillRing.rotate`, `KillRing.length` | `history.go` `KillRing` methods | direct | Kill-ring mutation and rotation semantics are represented. |
| `StdinBuffer` | `StdinBuffer.constructor`, `StdinBuffer.process`, `StdinBuffer.emitDataSequence`, `StdinBuffer.flush`, `StdinBuffer.clear`, `StdinBuffer.getBuffer`, `StdinBuffer.destroy` | `stdin_buffer.go` `StdinBuffer` methods | direct | Sequence buffering, paste handling, flush/clear/destroy, and event emission are represented. |
| `ProcessTerminal` | `ProcessTerminal.writeLogPath`, `ProcessTerminal.kittyProtocolActive`, `ProcessTerminal.start`, `ProcessTerminal.setupStdinBuffer`, `ProcessTerminal.queryAndEnableKittyProtocol`, `ProcessTerminal.enableWindowsVTInput`, `ProcessTerminal.drainInput`, `ProcessTerminal.stop`, `ProcessTerminal.write`, `ProcessTerminal.columns`, `ProcessTerminal.rows`, `ProcessTerminal.moveBy`, `ProcessTerminal.hideCursor`, `ProcessTerminal.showCursor`, `ProcessTerminal.clearLine`, `ProcessTerminal.clearFromCursor`, `ProcessTerminal.clearScreen`, `ProcessTerminal.setTitle`, `ProcessTerminal.setProgress`, `ProcessTerminal.clearProgressInterval` | `terminal.go`, `terminal_native.go`, `terminal_resize_*.go` | go-native | Go handles terminal IO natively while preserving raw mode, stdin buffering, Kitty negotiation, cursor, title, progress, and resize behavior. |
| `Container` | `Container.addChild`, `Container.removeChild`, `Container.clear`, `Container.invalidate`, `Container.render` | `tui.go` `Container` methods | direct | TUI root container mutation, invalidation, and render behavior are represented. |
| `TUI` | `TUI.constructor`, `TUI.fullRedraws`, `TUI.getShowHardwareCursor`, `TUI.setShowHardwareCursor`, `TUI.getClearOnShrink`, `TUI.setClearOnShrink`, `TUI.setFocus`, `TUI.showOverlay`, `TUI.hideOverlay`, `TUI.hasOverlay`, `TUI.isOverlayVisible`, `TUI.getTopmostVisibleOverlay`, `TUI.invalidate`, `TUI.start`, `TUI.addInputListener`, `TUI.removeInputListener`, `TUI.queryCellSize`, `TUI.stop`, `TUI.requestRender`, `TUI.scheduleRender`, `TUI.handleInput`, `TUI.consumeCellSizeResponse`, `TUI.resolveOverlayLayout`, `TUI.resolveAnchorRow`, `TUI.resolveAnchorCol`, `TUI.compositeOverlays`, `TUI.applyLineResets`, `TUI.collectKittyImageIds`, `TUI.deleteKittyImages`, `TUI.expandLastChangedForKittyImages`, `TUI.deleteChangedKittyImages`, `TUI.compositeLineAt`, `TUI.extractCursorPosition`, `TUI.doRender`, `TUI.positionHardwareCursor` | `tui.go`, `virtual_terminal.go` | direct | Start/stop, focus/input, overlays, diff/full redraw, cursor positioning, line reset, Kitty image cleanup, and hardware cursor options are represented. |
| `UndoStack` | `UndoStack.push`, `UndoStack.pop`, `UndoStack.clear`, `UndoStack.length` | `history.go` `UndoStack` methods | direct | Undo stack mutation semantics are represented. |
| `AnsiCodeTracker` | `AnsiCodeTracker.process`, `AnsiCodeTracker.reset`, `AnsiCodeTracker.clear`, `AnsiCodeTracker.getActiveCodes`, `AnsiCodeTracker.hasActiveCodes`, `AnsiCodeTracker.getLineEndReset` | `utils.go` `AnsiCodeTracker` methods | direct | ANSI style tracking and line-end reset behavior are represented. |

## Remaining TUI Gaps / Risks

- Markdown is the main implementation-shape risk: Pi uses `marked`, while Gi
  uses goldmark parsing plus custom terminal rendering. Continue comparing
  fixture behavior instead of assuming parser equivalence.
- `VirtualTerminal` is a Gi implementation used to test Pi-compatible TUI
  behavior. It is intentionally more like a headless xterm harness than a Pi
  source file; keep xterm behavior under CSI/OSC/resize/scrollback tests.
- A one-by-one Pi test-case to Gi test-case table is still needed. The current
  file map proves source ownership, not exhaustive test-case parity.

## Verification Evidence

- `go test ./gi-tui` is the focused validation gate for this map.
- `go test -timeout 30s ./...` passed after the latest TUI/coding-agent parity
  changes when allowed to bind localhost for OAuth callback tests.
