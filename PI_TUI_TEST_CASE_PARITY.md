# Pi TUI Test Case Parity

Generated from Pi `packages/tui/test` by extracting every top-level `it(...)` / `test(...)` case and mapping it to Gi TUI Go test coverage.

## Summary

- Pi explicit test cases: `592`
- Gi top-level TUI tests: `447`
- Mapped Pi cases: `592`
- Unmapped Pi cases: `0`
- Mapping references to missing Go tests: `0`

Notes: Gi tests intentionally group many Pi cases into table-driven or broader behavior tests, so this is a semantic case-to-coverage map, not a 1:1 test-name port.

## Test-folder Files Without Explicit Cases

| Pi file | Gi coverage |
|---|---|
| `chat-simple.ts` | `TestTUIChatSimpleDemoFlow` |
| `image-test.ts` | `TestImagePiManualDemoLayoutFallback` |
| `key-tester.ts` | `TestTUIKeyTesterDemoFlow` |
| `test-themes.ts` | `shared fixtures in components/tui/terminal_image tests` |
| `viewport-overwrite-repro.ts` | `TestTUIAppendPastViewportScrollsFromCurrentCursor` |
| `virtual-terminal.ts` | `VirtualTerminal API plus TestVirtualTerminal... harness tests` |

## `autocomplete.test.ts`

Pi cases: `25`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 59 | extracts / from 'hey /' when forced | `TestCombinedAutocompleteForcedPathExtractionPiMatrix`, `TestCombinedAutocompleteForcedAbsolutePathAtLineStart` | covered |
| 73 | extracts /A from '/A' when forced | `TestCombinedAutocompleteForcedPathExtractionPiMatrix`, `TestCombinedAutocompleteForcedAbsolutePathAtLineStart` | covered |
| 89 | does not trigger for slash commands | `TestCombinedAutocompleteSlashCommands` | covered |
| 101 | triggers for absolute paths after slash command argument | `TestCombinedAutocompleteForcedPathExtractionPiMatrix`, `TestCombinedAutocompleteForcedAbsolutePathAtLineStart` | covered |
| 134 | returns all files and folders for empty @ query | `TestCombinedAutocompleteFileSuggestions`, `TestCombinedAutocompleteFuzzyFullPathAndRelativeScope` | covered |
| 150 | matches file with extension in query | `TestCombinedAutocompleteFileSuggestions`, `TestCombinedAutocompleteFuzzyFullPathAndRelativeScope` | covered |
| 165 | filters are case insensitive | `TestCombinedAutocompleteFileSuggestions`, `TestCombinedAutocompleteFuzzyFullPathAndRelativeScope` | covered |
| 181 | ranks directories before files | `TestCombinedAutocompleteFileSuggestions`, `TestCombinedAutocompleteFuzzyFullPathAndRelativeScope` | covered |
| 199 | returns nested file paths | `TestCombinedAutocompleteFileSuggestions`, `TestCombinedAutocompleteFuzzyFullPathAndRelativeScope` | covered |
| 214 | matches deeply nested paths | `TestCombinedAutocompleteFileSuggestions`, `TestCombinedAutocompleteFuzzyFullPathAndRelativeScope` | covered |
| 231 | matches directory in middle of path with --full-path | `TestCombinedAutocompleteFileSuggestions`, `TestCombinedAutocompleteFuzzyFullPathAndRelativeScope` | covered |
| 248 | scopes fuzzy search to relative directories and searches recursively | `TestCombinedAutocompleteFileSuggestions`, `TestCombinedAutocompleteFuzzyFullPathAndRelativeScope` | covered |
| 267 | quotes paths with spaces for @ suggestions | `TestCombinedAutocompleteFileSuggestions`, `TestCombinedAutocompleteFuzzyFullPathAndRelativeScope` | covered |
| 283 | includes hidden paths but excludes .git | `TestCombinedAutocompleteFileSuggestions`, `TestCombinedAutocompleteFuzzyFullPathAndRelativeScope` | covered |
| 303 | follows symlinked directories for fuzzy @ search | `TestCombinedAutocompleteFollowsSymlinksLikeFd` | covered |
| 325 | returns symlinked directories when matching their name | `TestCombinedAutocompleteFollowsSymlinksLikeFd` | covered |
| 341 | returns symlinked files without requiring type l | `TestCombinedAutocompleteFollowsSymlinksLikeFd` | covered |
| 358 | returns the same @ suggestions when the cwd path contains the query | `TestCombinedAutocompleteFuzzyResultsIgnoreQueryInBasePath` | covered |
| 391 | continues autocomplete inside quoted @ paths | `TestCombinedAutocompleteQuotedAndDotSlashCompletion` | covered |
| 409 | applies quoted @ completion without duplicating closing quote | `TestCombinedAutocompleteQuotedAndDotSlashCompletion` | covered |
| 441 | preserves ./ prefix when completing paths | `TestCombinedAutocompleteQuotedAndDotSlashCompletion` | covered |
| 458 | preserves ./ prefix for directory completions | `TestCombinedAutocompleteQuotedAndDotSlashCompletion` | covered |
| 487 | quotes paths with spaces for direct completion | `TestCombinedAutocompleteFileSuggestions` | covered |
| 504 | continues completion inside quoted paths | `TestCombinedAutocompleteQuotedAndDotSlashCompletion` | covered |
| 522 | applies quoted completion without duplicating closing quote | `TestCombinedAutocompleteQuotedAndDotSlashCompletion` | covered |

## `bug-regression-isimageline-startswith-bug.test.ts`

Pi cases: `11`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 20 | old implementation would return false, causing crash | `TestIsImageLineDetectsKittyAndITermAnywhere`, `TestIsImageLinePiCoverageMatrix` | covered |
| 58 | new implementation returns true correctly | `TestIsImageLineDetectsKittyAndITermAnywhere`, `TestIsImageLinePiCoverageMatrix` | covered |
| 70 | new implementation detects Kitty sequences in any position | `TestIsImageLineDetectsKittyAndITermAnywhere`, `TestIsImageLinePiCoverageMatrix` | covered |
| 87 | new implementation detects iTerm2 sequences in any position | `TestIsImageLineDetectsKittyAndITermAnywhere`, `TestIsImageLinePiCoverageMatrix` | covered |
| 123 | detects image sequences in read tool output | `TestIsImageLineDetectsKittyAndITermAnywhere`, `TestIsImageLinePiCoverageMatrix` | covered |
| 133 | detects Kitty sequences from Image component | `TestIsImageLineDetectsKittyAndITermAnywhere`, `TestIsImageLinePiCoverageMatrix` | covered |
| 142 | handles ANSI codes before image sequences | `TestIsImageLineDetectsKittyAndITermAnywhere`, `TestIsImageLinePiCoverageMatrix` | covered |
| 163 | does NOT crash on very long lines with image sequences | `TestIsImageLineDetectsKittyAndITermAnywhere`, `TestIsImageLinePiCoverageMatrix` | covered |
| 192 | handles lines exactly matching crash log dimensions | `TestIsImageLineDetectsKittyAndITermAnywhere`, `TestIsImageLinePiCoverageMatrix` | covered |
| 213 | does not detect images in regular long text | `TestIsImageLineDetectsKittyAndITermAnywhere`, `TestIsImageLinePiCoverageMatrix` | covered |
| 222 | does not detect images in lines with file paths | `TestIsImageLineDetectsKittyAndITermAnywhere`, `TestIsImageLinePiCoverageMatrix` | covered |

## `editor.test.ts`

Pi cases: `175`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 43 | does nothing on Up arrow when history is empty | `TestEditorPromptHistoryNavigation`, `TestEditorHistoryExitsOnTypingAndSkipsDuplicates`, `TestEditorHistoryLimitKeepsMostRecentHundredEntries` | covered |
| 51 | shows most recent history entry on Up arrow when editor is empty | `TestEditorPromptHistoryNavigation`, `TestEditorHistoryExitsOnTypingAndSkipsDuplicates`, `TestEditorHistoryLimitKeepsMostRecentHundredEntries` | covered |
| 62 | cycles through history entries on repeated Up arrow | `TestEditorPromptHistoryNavigation`, `TestEditorHistoryExitsOnTypingAndSkipsDuplicates`, `TestEditorHistoryLimitKeepsMostRecentHundredEntries` | covered |
| 82 | returns to empty editor on Down arrow after browsing history | `TestEditorPromptHistoryNavigation`, `TestEditorHistoryExitsOnTypingAndSkipsDuplicates`, `TestEditorHistoryLimitKeepsMostRecentHundredEntries` | covered |
| 94 | navigates forward through history with Down arrow | `TestEditorPromptHistoryNavigation`, `TestEditorHistoryExitsOnTypingAndSkipsDuplicates`, `TestEditorHistoryLimitKeepsMostRecentHundredEntries` | covered |
| 117 | exits history mode when typing a character | `TestEditorPromptHistoryNavigation`, `TestEditorHistoryExitsOnTypingAndSkipsDuplicates`, `TestEditorHistoryLimitKeepsMostRecentHundredEntries` | covered |
| 128 | exits history mode on setText | `TestEditorPromptHistoryNavigation`, `TestEditorHistoryExitsOnTypingAndSkipsDuplicates`, `TestEditorHistoryLimitKeepsMostRecentHundredEntries` | covered |
| 142 | does not add empty strings to history | `TestEditorPromptHistoryNavigation`, `TestEditorHistoryExitsOnTypingAndSkipsDuplicates`, `TestEditorHistoryLimitKeepsMostRecentHundredEntries` | covered |
| 157 | does not add consecutive duplicates to history | `TestEditorPromptHistoryNavigation`, `TestEditorHistoryExitsOnTypingAndSkipsDuplicates`, `TestEditorHistoryLimitKeepsMostRecentHundredEntries` | covered |
| 171 | allows non-consecutive duplicates in history | `TestEditorPromptHistoryNavigation`, `TestEditorHistoryExitsOnTypingAndSkipsDuplicates`, `TestEditorHistoryLimitKeepsMostRecentHundredEntries` | covered |
| 188 | uses cursor movement instead of history when editor has content | `TestEditorPromptHistoryNavigation`, `TestEditorHistoryExitsOnTypingAndSkipsDuplicates`, `TestEditorHistoryLimitKeepsMostRecentHundredEntries` | covered |
| 204 | limits history to 100 entries | `TestEditorPromptHistoryNavigation`, `TestEditorHistoryExitsOnTypingAndSkipsDuplicates`, `TestEditorHistoryLimitKeepsMostRecentHundredEntries` | covered |
| 225 | allows cursor movement within multi-line history entry with Down | `TestEditorHistoryUsesWrappedVisualLineBoundaries` | covered |
| 239 | allows cursor movement within multi-line history entry with Up | `TestEditorHistoryUsesWrappedVisualLineBoundaries` | covered |
| 260 | navigates from multi-line entry back to newer via Down after cursor movement | `TestEditorRenderUsesPiBordersAndCursor` | covered |
| 284 | returns cursor position | `TestEditorPublicAccessorsReturnCursorAndLines` | covered |
| 299 | returns lines as a defensive copy | `TestEditorPublicAccessorsReturnCursorAndLines` | covered |
| 312 | inserts backslash immediately (no buffering) | `TestEditorBackslashEnterNewlineWorkaround` | covered |
| 321 | converts standalone backslash to newline on Enter | `TestEditorBackslashEnterNewlineWorkaround` | covered |
| 330 | inserts backslash normally when followed by other characters | `TestEditorBackslashEnterNewlineWorkaround` | covered |
| 339 | does not trigger newline when backslash is not immediately before cursor | `TestEditorBackslashEnterNewlineWorkaround` | covered |
| 355 | only removes one backslash when multiple are present | `TestEditorBackslashEnterNewlineWorkaround` | covered |
| 370 | ignores printable CSI-u sequences with unsupported modifiers | `TestEditorCSIuPrintableInput`, `TestEditorBracketedPasteDecodesCSIuControls` | covered |
| 378 | inserts shifted CSI-u letters as text | `TestEditorCSIuPrintableInput`, `TestEditorBracketedPasteDecodesCSIuControls` | covered |
| 386 | inserts shifted xterm modifyOtherKeys letters as text | `TestEditorCSIuPrintableInput`, `TestEditorBracketedPasteDecodesCSIuControls` | covered |
| 396 | inserts mixed ASCII, umlauts, and emojis as literal text | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 415 | deletes single-code-unit unicode characters (umlauts) with Backspace | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 429 | deletes multi-code-unit emojis with single Backspace | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 442 | inserts characters at the correct position after cursor movement over umlauts | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 460 | moves cursor across multi-code-unit emojis with single arrow key | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 480 | preserves umlauts across line breaks | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 495 | replaces the entire document with unicode text via setText (paste simulation) | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 505 | moves cursor to document start on Ctrl+A and inserts at the beginning | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 517 | deletes words correctly with Ctrl+W and Alt+Backspace | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 558 | navigates words correctly with Ctrl+Left/Right | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 597 | wraps lines correctly when text contains wide emojis | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 612 | wraps long text with emojis at correct positions | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 629 | renders isolated Thai and Lao AM clusters without width drift | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 641 | wraps CJK characters correctly (each is 2 columns wide) | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 661 | handles mixed ASCII and wide characters in wrapping | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 677 | renders cursor correctly on wide characters | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 693 | does not exceed terminal width with emoji at wrap boundary | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 708 | shows cursor at end of line before wrap, wraps on next char | `TestWordWrapLinePiBoundaryCases`, `TestWordWrapLineWideCharAfterWrapOpportunity`, `TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi`, `TestEditorRenderLongWordsAndLeadingWhitespaceLikePi` | covered |
| 730 | wraps at word boundaries instead of mid-word | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 752 | does not start lines with leading whitespace after word wrap | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 773 | breaks long words (URLs) at character level | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 787 | preserves multiple spaces within words on same line | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 799 | handles empty string | `TestEditorRenderUsesPiBordersAndCursor` | covered |
| 810 | handles single word that fits exactly | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 823 | wraps word to next line when it ends exactly at terminal width | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 833 | keeps whitespace at terminal width boundary on same line | `TestWordWrapLinePiBoundaryCases`, `TestWordWrapLineWideCharAfterWrapOpportunity`, `TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi`, `TestEditorRenderLongWordsAndLeadingWhitespaceLikePi` | covered |
| 843 | handles unbreakable word filling width exactly followed by space | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 851 | wraps word to next line when it fits width but not remaining space | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 859 | keeps word with multi-space and following word together when they fit | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 867 | keeps word with multi-space and following word when they fill width exactly | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 875 | splits when word plus multi-space plus word exceeds width | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 884 | breaks long whitespace at line boundary | `TestWordWrapLinePiBoundaryCases`, `TestWordWrapLineWideCharAfterWrapOpportunity`, `TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi`, `TestEditorRenderLongWordsAndLeadingWhitespaceLikePi` | covered |
| 893 | breaks long whitespace at line boundary 2 | `TestWordWrapLinePiBoundaryCases`, `TestWordWrapLineWideCharAfterWrapOpportunity`, `TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi`, `TestEditorRenderLongWordsAndLeadingWhitespaceLikePi` | covered |
| 902 | breaks whitespace spanning full lines | `TestWordWrapLinePiBoundaryCases`, `TestWordWrapLineWideCharAfterWrapOpportunity`, `TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi`, `TestEditorRenderLongWordsAndLeadingWhitespaceLikePi` | covered |
| 911 | force-breaks when wide char after word boundary wrap still overflows | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 929 | splits oversized atomic segment across multiple chunks | `TestWordWrapLinePiBoundaryCases`, `TestWordWrapLineWideCharAfterWrapOpportunity`, `TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi`, `TestEditorRenderLongWordsAndLeadingWhitespaceLikePi` | covered |
| 954 | splits oversized atomic segment at start of line | `TestWordWrapLinePiBoundaryCases`, `TestWordWrapLineWideCharAfterWrapOpportunity`, `TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi`, `TestEditorRenderLongWordsAndLeadingWhitespaceLikePi` | covered |
| 974 | splits oversized atomic segment at end of line | `TestWordWrapLinePiBoundaryCases`, `TestWordWrapLineWideCharAfterWrapOpportunity`, `TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi`, `TestEditorRenderLongWordsAndLeadingWhitespaceLikePi` | covered |
| 993 | splits consecutive oversized atomic segments | `TestWordWrapLinePiBoundaryCases`, `TestWordWrapLineWideCharAfterWrapOpportunity`, `TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi`, `TestEditorRenderLongWordsAndLeadingWhitespaceLikePi` | covered |
| 1015 | wraps normally after oversized atomic segment | `TestWordWrapLinePiBoundaryCases`, `TestWordWrapLineWideCharAfterWrapOpportunity`, `TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi`, `TestEditorRenderLongWordsAndLeadingWhitespaceLikePi` | covered |
| 1053 | Ctrl+W saves deleted text to kill ring and Ctrl+Y yanks it | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 1066 | Ctrl+U saves deleted text to kill ring | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1086 | Ctrl+K saves deleted text to kill ring | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1099 | Ctrl+Y does nothing when kill ring is empty | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1107 | Alt+Y cycles through kill ring after Ctrl+Y | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1134 | Alt+Y does nothing if not preceded by yank | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1150 | Alt+Y does nothing if kill ring has ≤1 entry | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1163 | consecutive Ctrl+W accumulates into one kill ring entry | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 1178 | Ctrl+U accumulates multiline deletes including newlines | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1210 | backward deletions prepend, forward deletions append during accumulation | `TestEditorRenderUsesPiBordersAndCursor` | covered |
| 1226 | non-delete actions break kill accumulation | `TestEditorRenderUsesPiBordersAndCursor` | covered |
| 1249 | non-yank actions break Alt+Y chain | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1268 | kill ring rotation persists after cycling | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1296 | consecutive deletions across lines coalesce into one entry | `TestEditorRenderUsesPiBordersAndCursor` | covered |
| 1321 | Ctrl+K at line end deletes newline and coalesces | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1348 | handles yank in middle of text | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1363 | handles yank-pop in middle of text | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1388 | multiline yank and yank-pop in middle of text | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1416 | Alt+D deletes word forward and saves to kill ring | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 1433 | Alt+D at end of line deletes newline | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1450 | does nothing when undo stack is empty | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1457 | coalesces consecutive word characters into one undo unit | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 1482 | undoes spaces one at a time | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1504 | undoes newlines and signals next word to capture state | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 1530 | undoes backspace | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1545 | undoes forward delete | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1562 | undoes Ctrl+W (delete word backward) | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 1585 | undoes Ctrl+K (delete to line end) | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1612 | undoes Ctrl+U (delete to line start) | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1636 | undoes yank | `TestEditorKillRingPiAccumulationAndYankPlacement`, `TestEditorKillRingYankPopRotationPersists` | covered |
| 1653 | undoes single-line paste atomically | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1672 | does not trigger autocomplete during single-line paste | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 1692 | decodes CSI-u Ctrl+letter sequences inside bracketed paste (tmux popup) | `TestEditorCSIuPrintableInput`, `TestEditorBracketedPasteDecodesCSIuControls` | covered |
| 1702 | undoes multi-line paste atomically | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1721 | undoes insertTextAtCursor atomically | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1740 | insertTextAtCursor handles multiline text | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1761 | insertTextAtCursor normalizes CRLF and CR line endings | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1778 | undoes setText to empty string | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1801 | clears undo stack on submit | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1823 | exits history browsing mode on undo | `TestEditorUndoExitsHistoryBrowsingToPreHistoryState`, `TestEditorUndoSkipsIntermediateHistoryNavigationStates` | covered |
| 1855 | undo restores to pre-history state even after multiple history navigations | `TestEditorUndoExitsHistoryBrowsingToPreHistoryState`, `TestEditorUndoSkipsIntermediateHistoryNavigationStates` | covered |
| 1894 | cursor movement starts new undo unit | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1927 | no-op delete operations do not push undo snapshots | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1948 | undoes autocomplete | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 1987 | auto-applies single force-file suggestion without showing menu | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2028 | shows menu when force-file has multiple suggestions | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2072 | keeps suggestions open when typing in force mode (Tab-triggered) | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2124 | debounces @ autocomplete while typing | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2157 | debounces # autocomplete while typing | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2190 | aborts active @ autocomplete when typing continues | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2227 | hides autocomplete when backspacing slash command to empty | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2267 | applies exact typed slash-argument value on Enter even when first item is highlighted | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2323 | selects first prefix match on Enter when typed arg is not exact match | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2374 | highlights unique prefix match as user types (before full exact match) | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2423 | selects first prefix match when multiple items match | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2469 | works for built-in-style command argument completion path (model-like) | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2532 | awaits async slash command argument completions | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2557 | ignores invalid slash command argument completion results | `TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo`, `TestEditorAutocompleteCombinedProviderUsesChildProviders`, `TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu`, `TestEditorAutocompleteSelectsBestPrefixMatch`, `TestEditorAutocompleteRetainsExactTypedSlashArgument`, `TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument`, `TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions` | covered |
| 2580 | does not show argument completions when command has no argument completer | `TestEditorRenderUsesPiBordersAndCursor` | covered |
| 2608 | jumps forward to first occurrence of character on same line | `TestEditorCharacterJump`, `TestEditorCharacterJumpPiEdgeCases` | covered |
| 2621 | jumps forward to next occurrence after cursor | `TestEditorCharacterJump`, `TestEditorCharacterJumpPiEdgeCases` | covered |
| 2636 | jumps forward across multiple lines | `TestEditorCharacterJump`, `TestEditorCharacterJumpPiEdgeCases` | covered |
| 2652 | jumps backward to first occurrence before cursor on same line | `TestEditorCharacterJump`, `TestEditorCharacterJumpPiEdgeCases` | covered |
| 2665 | jumps backward across multiple lines | `TestEditorCharacterJump`, `TestEditorCharacterJumpPiEdgeCases` | covered |
| 2678 | does nothing when character is not found (forward) | `TestEditorRenderUsesPiBordersAndCursor` | covered |
| 2691 | does nothing when character is not found (backward) | `TestEditorRenderUsesPiBordersAndCursor` | covered |
| 2704 | is case-sensitive | `TestEditorCharacterJump`, `TestEditorCharacterJumpPiEdgeCases` | covered |
| 2724 | cancels jump mode when Ctrl+] is pressed again | `TestEditorCharacterJump`, `TestEditorCharacterJumpPiEdgeCases` | covered |
| 2739 | cancels jump mode on Escape and processes the Escape | `TestEditorCharacterJump`, `TestEditorCharacterJumpPiEdgeCases` | covered |
| 2757 | cancels backward jump mode when Ctrl+Alt+] is pressed again | `TestEditorCharacterJump`, `TestEditorCharacterJumpPiEdgeCases` | covered |
| 2772 | searches for special characters | `TestEditorCharacterJump`, `TestEditorCharacterJumpPiEdgeCases` | covered |
| 2792 | handles empty text gracefully | `TestEditorCharacterJump`, `TestEditorCharacterJumpPiEdgeCases` | covered |
| 2804 | resets lastAction when jumping | `TestEditorCharacterJump`, `TestEditorCharacterJumpPiEdgeCases` | covered |
| 2840 | preserves target column when moving up through a shorter line | `TestEditorStickyColumnForLogicalLineMovement`, `TestEditorStickyColumnResetsOnEditingAndNavigationLikePi`, `TestEditorStickyColumnRewrapsAfterResize` | covered |
| 2863 | preserves target column when moving down through a shorter line | `TestEditorStickyColumnForLogicalLineMovement`, `TestEditorStickyColumnResetsOnEditingAndNavigationLikePi`, `TestEditorStickyColumnRewrapsAfterResize` | covered |
| 2884 | resets sticky column on horizontal movement (left arrow) | `TestEditorStickyColumnForLogicalLineMovement`, `TestEditorStickyColumnResetsOnEditingAndNavigationLikePi`, `TestEditorStickyColumnRewrapsAfterResize` | covered |
| 2909 | resets sticky column on horizontal movement (right arrow) | `TestEditorStickyColumnForLogicalLineMovement`, `TestEditorStickyColumnResetsOnEditingAndNavigationLikePi`, `TestEditorStickyColumnRewrapsAfterResize` | covered |
| 2936 | resets sticky column on typing | `TestEditorStickyColumnForLogicalLineMovement`, `TestEditorStickyColumnResetsOnEditingAndNavigationLikePi`, `TestEditorStickyColumnRewrapsAfterResize` | covered |
| 2960 | resets sticky column on backspace | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 2984 | resets sticky column on Ctrl+A (move to line start) | `TestEditorStickyColumnForLogicalLineMovement`, `TestEditorStickyColumnResetsOnEditingAndNavigationLikePi`, `TestEditorStickyColumnRewrapsAfterResize` | covered |
| 3005 | resets sticky column on Ctrl+E (move to line end) | `TestEditorStickyColumnForLogicalLineMovement`, `TestEditorStickyColumnResetsOnEditingAndNavigationLikePi`, `TestEditorStickyColumnRewrapsAfterResize` | covered |
| 3029 | resets sticky column on word movement (Ctrl+Left) | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 3052 | resets sticky column on word movement (Ctrl+Right) | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 3078 | resets sticky column on undo | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 3116 | handles multiple consecutive up/down movements | `TestEditorStickyColumnForLogicalLineMovement`, `TestEditorStickyColumnResetsOnEditingAndNavigationLikePi`, `TestEditorStickyColumnRewrapsAfterResize` | covered |
| 3141 | moves correctly through wrapped visual lines without getting stuck | `TestWordWrapLinePiBoundaryCases`, `TestWordWrapLineWideCharAfterWrapOpportunity`, `TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi`, `TestEditorRenderLongWordsAndLeadingWhitespaceLikePi` | covered |
| 3165 | handles setText resetting sticky column | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 3185 | sets preferredVisualCol when pressing right at end of prompt (last line) | `TestEditorStickyColumnForLogicalLineMovement`, `TestEditorStickyColumnResetsOnEditingAndNavigationLikePi`, `TestEditorStickyColumnRewrapsAfterResize` | covered |
| 3214 | handles editor resizes when preferredVisualCol is on the same line | `TestEditorStickyColumnForLogicalLineMovement`, `TestEditorStickyColumnResetsOnEditingAndNavigationLikePi`, `TestEditorStickyColumnRewrapsAfterResize` | covered |
| 3239 | handles editor resizes when preferredVisualCol is on a different line | `TestEditorStickyColumnForLogicalLineMovement`, `TestEditorStickyColumnResetsOnEditingAndNavigationLikePi`, `TestEditorStickyColumnRewrapsAfterResize` | covered |
| 3277 | rewrapped lines: target fits current visual column | `TestWordWrapLinePiBoundaryCases`, `TestWordWrapLineWideCharAfterWrapOpportunity`, `TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi`, `TestEditorRenderLongWordsAndLeadingWhitespaceLikePi` | covered |
| 3303 | rewrapped lines: target shorter than current visual column | `TestWordWrapLinePiBoundaryCases`, `TestWordWrapLineWideCharAfterWrapOpportunity`, `TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi`, `TestEditorRenderLongWordsAndLeadingWhitespaceLikePi` | covered |
| 3339 | creates a paste marker for large pastes | `TestEditorLargePasteMarkerExpansionAndSubmit`, `TestEditorPasteMarkerAtomicNavigationAndDelete`, `TestEditorPasteMarkerAtomicWordMovementAndManualMarker`, `TestEditorPasteMarkerVerticalMovementSnapsToMarkerStart`, `TestEditorPasteMarkerVerticalMovementPreservesStickyColumn`, `TestEditorPasteMarkerVerticalMovementThroughWrappedMarker`, `TestEditorPasteMarkerVerticalMovementSkipsMarkerContinuationTail`, `TestEditorPasteMarkerRenderUsesMarkerAwareVisualChunks` | covered |
| 3345 | treats paste marker as single unit for right arrow | `TestEditorLargePasteMarkerExpansionAndSubmit`, `TestEditorPasteMarkerAtomicNavigationAndDelete`, `TestEditorPasteMarkerAtomicWordMovementAndManualMarker`, `TestEditorPasteMarkerVerticalMovementSnapsToMarkerStart`, `TestEditorPasteMarkerVerticalMovementPreservesStickyColumn`, `TestEditorPasteMarkerVerticalMovementThroughWrappedMarker`, `TestEditorPasteMarkerVerticalMovementSkipsMarkerContinuationTail`, `TestEditorPasteMarkerRenderUsesMarkerAwareVisualChunks` | covered |
| 3370 | treats paste marker as single unit for left arrow | `TestEditorLargePasteMarkerExpansionAndSubmit`, `TestEditorPasteMarkerAtomicNavigationAndDelete`, `TestEditorPasteMarkerAtomicWordMovementAndManualMarker`, `TestEditorPasteMarkerVerticalMovementSnapsToMarkerStart`, `TestEditorPasteMarkerVerticalMovementPreservesStickyColumn`, `TestEditorPasteMarkerVerticalMovementThroughWrappedMarker`, `TestEditorPasteMarkerVerticalMovementSkipsMarkerContinuationTail`, `TestEditorPasteMarkerRenderUsesMarkerAwareVisualChunks` | covered |
| 3392 | treats paste marker as single unit for backspace | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 3414 | treats paste marker as single unit for forward delete | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 3430 | treats paste marker as single unit for word movement | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 3454 | undo restores marker after backspace deletion | `TestEditorUndoTypingCoalescesWordsAndSpaces`, `TestEditorUndoNewlineSplitsTypingUnitsLikePi`, `TestEditorUndoDeleteKillAndYank`, `TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries`, `TestEditorBracketedPasteAndUndoAreAtomic` | covered |
| 3476 | handles multiple paste markers in same line | `TestEditorLargePasteMarkerExpansionAndSubmit`, `TestEditorPasteMarkerAtomicNavigationAndDelete`, `TestEditorPasteMarkerAtomicWordMovementAndManualMarker`, `TestEditorPasteMarkerVerticalMovementSnapsToMarkerStart`, `TestEditorPasteMarkerVerticalMovementPreservesStickyColumn`, `TestEditorPasteMarkerVerticalMovementThroughWrappedMarker`, `TestEditorPasteMarkerVerticalMovementSkipsMarkerContinuationTail`, `TestEditorPasteMarkerRenderUsesMarkerAwareVisualChunks` | covered |
| 3505 | does not treat manually typed marker-like text as atomic (no valid paste ID) | `TestEditorLargePasteMarkerExpansionAndSubmit`, `TestEditorPasteMarkerAtomicNavigationAndDelete`, `TestEditorPasteMarkerAtomicWordMovementAndManualMarker`, `TestEditorPasteMarkerVerticalMovementSnapsToMarkerStart`, `TestEditorPasteMarkerVerticalMovementPreservesStickyColumn`, `TestEditorPasteMarkerVerticalMovementThroughWrappedMarker`, `TestEditorPasteMarkerVerticalMovementSkipsMarkerContinuationTail`, `TestEditorPasteMarkerRenderUsesMarkerAwareVisualChunks` | covered |
| 3520 | does not crash when paste marker is wider than terminal width | `TestEditorUnicodeTextEditingMatchesPi`, `TestEditorGraphemeClusterNavigationAndDeletion`, `TestEditorRenderWideUnicodeLinesFitWidth` | covered |
| 3543 | does not crash when text + paste marker exceeds terminal width with cursor on marker | `TestEditorLargePasteMarkerExpansionAndSubmit`, `TestEditorPasteMarkerAtomicNavigationAndDelete`, `TestEditorPasteMarkerAtomicWordMovementAndManualMarker`, `TestEditorPasteMarkerVerticalMovementSnapsToMarkerStart`, `TestEditorPasteMarkerVerticalMovementPreservesStickyColumn`, `TestEditorPasteMarkerVerticalMovementThroughWrappedMarker`, `TestEditorPasteMarkerVerticalMovementSkipsMarkerContinuationTail`, `TestEditorPasteMarkerRenderUsesMarkerAwareVisualChunks` | covered |
| 3578 | wordWrapLine re-checks overflow after backtracking to wrap opportunity | `TestEditorPunctuationAwareWordDeletionAndNavigation` | covered |
| 3607 | expands large pasted content literally in getExpandedText | `TestEditorLargePasteMarkerExpansionAndSubmit`, `TestEditorPasteMarkerAtomicNavigationAndDelete`, `TestEditorPasteMarkerAtomicWordMovementAndManualMarker`, `TestEditorPasteMarkerVerticalMovementSnapsToMarkerStart`, `TestEditorPasteMarkerVerticalMovementPreservesStickyColumn`, `TestEditorPasteMarkerVerticalMovementThroughWrappedMarker`, `TestEditorPasteMarkerVerticalMovementSkipsMarkerContinuationTail`, `TestEditorPasteMarkerRenderUsesMarkerAwareVisualChunks` | covered |
| 3629 | snaps to the paste marker start when navigating down into it | `TestEditorLargePasteMarkerExpansionAndSubmit`, `TestEditorPasteMarkerAtomicNavigationAndDelete`, `TestEditorPasteMarkerAtomicWordMovementAndManualMarker`, `TestEditorPasteMarkerVerticalMovementSnapsToMarkerStart`, `TestEditorPasteMarkerVerticalMovementPreservesStickyColumn`, `TestEditorPasteMarkerVerticalMovementThroughWrappedMarker`, `TestEditorPasteMarkerVerticalMovementSkipsMarkerContinuationTail`, `TestEditorPasteMarkerRenderUsesMarkerAwareVisualChunks` | covered |
| 3664 | preserves sticky column when navigating through paste marker line | `TestEditorStickyColumnForLogicalLineMovement`, `TestEditorStickyColumnResetsOnEditingAndNavigationLikePi`, `TestEditorStickyColumnRewrapsAfterResize` | covered |
| 3706 | does not get stuck moving down from a multi-visual-line paste marker | `TestEditorLargePasteMarkerExpansionAndSubmit`, `TestEditorPasteMarkerAtomicNavigationAndDelete`, `TestEditorPasteMarkerAtomicWordMovementAndManualMarker`, `TestEditorPasteMarkerVerticalMovementSnapsToMarkerStart`, `TestEditorPasteMarkerVerticalMovementPreservesStickyColumn`, `TestEditorPasteMarkerVerticalMovementThroughWrappedMarker`, `TestEditorPasteMarkerVerticalMovementSkipsMarkerContinuationTail`, `TestEditorPasteMarkerRenderUsesMarkerAwareVisualChunks` | covered |
| 3767 | skips marker continuation VLs when preferred col falls in marker tail | `TestEditorLargePasteMarkerExpansionAndSubmit`, `TestEditorPasteMarkerAtomicNavigationAndDelete`, `TestEditorPasteMarkerAtomicWordMovementAndManualMarker`, `TestEditorPasteMarkerVerticalMovementSnapsToMarkerStart`, `TestEditorPasteMarkerVerticalMovementPreservesStickyColumn`, `TestEditorPasteMarkerVerticalMovementThroughWrappedMarker`, `TestEditorPasteMarkerVerticalMovementSkipsMarkerContinuationTail`, `TestEditorPasteMarkerRenderUsesMarkerAwareVisualChunks` | covered |
| 3808 | submits large pasted content literally | `TestEditorLargePasteMarkerExpansionAndSubmit`, `TestEditorPasteMarkerAtomicNavigationAndDelete`, `TestEditorPasteMarkerAtomicWordMovementAndManualMarker`, `TestEditorPasteMarkerVerticalMovementSnapsToMarkerStart`, `TestEditorPasteMarkerVerticalMovementPreservesStickyColumn`, `TestEditorPasteMarkerVerticalMovementThroughWrappedMarker`, `TestEditorPasteMarkerVerticalMovementSkipsMarkerContinuationTail`, `TestEditorPasteMarkerRenderUsesMarkerAwareVisualChunks` | covered |

## `fuzzy.test.ts`

Pi cases: `13`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 6 | empty query matches everything with score 0 | `TestFuzzyMatchPiSemantics` | covered |
| 12 | query longer than text does not match | `TestFuzzyMatchPiSemantics` | covered |
| 17 | exact match has good score | `TestFuzzyMatchPiSemantics` | covered |
| 23 | characters must appear in order | `TestFuzzyMatchPiSemantics` | covered |
| 31 | case insensitive matching | `TestFuzzyMatchPiSemantics` | covered |
| 39 | consecutive matches score better than scattered matches | `TestFuzzyMatchPiSemantics` | covered |
| 48 | word boundary matches score better | `TestFuzzyMatchPiSemantics` | covered |
| 57 | matches swapped alpha numeric tokens | `TestFuzzyMatchPiSemantics` | covered |
| 64 | empty query returns all items unchanged | `TestFuzzyFilterPiSemantics` | covered |
| 70 | filters out non-matching items | `TestFuzzyFilterPiSemantics` | covered |
| 78 | sorts results by match quality | `TestFuzzyFilterPiSemantics` | covered |
| 86 | prioritizes exact matches over longer prefix matches | `TestFuzzyMatchPiSemantics` | covered |
| 93 | works with custom getText function | `TestFuzzyFilterPiSemantics` | covered |

## `input.test.ts`

Pi cases: `31`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 7 | submits value including backslash on Enter | `TestInputEditingAndSubmit` | covered |
| 28 | inserts backslash as regular character | `TestInputEditingAndSubmit` | covered |
| 38 | does not overflow with wide CJK and fullwidth text | `TestInputRenderWideTextKeepsCursorVisible`, `TestInputPiWideTextRenderMatrix` | covered |
| 71 | keeps the cursor visible when horizontally scrolling wide text | `TestInputRenderWideTextKeepsCursorVisible`, `TestInputPiWideTextRenderMatrix` | covered |
| 87 | Ctrl+W saves deleted text to kill ring and Ctrl+Y yanks it | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 103 | Ctrl+U saves deleted text to kill ring | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 118 | Ctrl+K saves deleted text to kill ring | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 131 | Ctrl+Y does nothing when kill ring is empty | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 140 | Alt+Y cycles through kill ring after Ctrl+Y | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 169 | Alt+Y does nothing if not preceded by yank | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 186 | Alt+Y does nothing if kill ring has one entry | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 200 | consecutive Ctrl+W accumulates into one kill ring entry | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 215 | non-delete actions break kill accumulation | `TestInputPiUndoEdgeCases`, `TestInputUndoDeletePasteAndForwardWord`, `TestInputUndoReadlineDeletionShortcutsLikePi` | covered |
| 236 | non-yank actions break Alt+Y chain | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 257 | kill ring rotation persists after cycling | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 284 | backward deletions prepend, forward deletions append during accumulation | `TestInputEditingAndSubmit` | covered |
| 299 | Alt+D deletes word forward and saves to kill ring | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 316 | handles yank in middle of text | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 331 | handles yank-pop in middle of text | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 356 | does nothing when undo stack is empty | `TestInputPiUndoEdgeCases`, `TestInputUndoDeletePasteAndForwardWord`, `TestInputUndoReadlineDeletionShortcutsLikePi` | covered |
| 363 | coalesces consecutive word characters into one undo unit | `TestInputPiUndoEdgeCases`, `TestInputUndoDeletePasteAndForwardWord`, `TestInputUndoReadlineDeletionShortcutsLikePi` | covered |
| 388 | undoes spaces one at a time | `TestInputPiUndoEdgeCases`, `TestInputUndoDeletePasteAndForwardWord`, `TestInputUndoReadlineDeletionShortcutsLikePi` | covered |
| 410 | undoes backspace | `TestInputPiUndoEdgeCases`, `TestInputUndoDeletePasteAndForwardWord`, `TestInputUndoReadlineDeletionShortcutsLikePi` | covered |
| 425 | undoes forward delete | `TestInputPiUndoEdgeCases`, `TestInputUndoDeletePasteAndForwardWord`, `TestInputUndoReadlineDeletionShortcutsLikePi` | covered |
| 442 | undoes Ctrl+W (delete word backward) | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 465 | undoes Ctrl+K (delete to line end) | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 489 | undoes Ctrl+U (delete to line start) | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 513 | undoes yank | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 530 | undoes paste atomically | `TestInputPiUndoEdgeCases`, `TestInputUndoDeletePasteAndForwardWord`, `TestInputUndoReadlineDeletionShortcutsLikePi` | covered |
| 546 | undoes Alt+D (delete word forward) | `TestInputKillRingReadlineShortcuts`, `TestInputPiKillRingEdgeCases`, `TestInputYankPopCyclesKillRing` | covered |
| 559 | cursor movement starts new undo unit | `TestInputPiUndoEdgeCases`, `TestInputUndoDeletePasteAndForwardWord`, `TestInputUndoReadlineDeletionShortcutsLikePi` | covered |

## `keybindings.test.ts`

Pi cases: `3`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 6 | does not evict selector confirm when input submit is rebound | `TestKeybindingsManagerDoesNotEvictDefaultsWhenUserReusesKeys` | covered |
| 15 | does not evict cursor bindings when another action reuses the same key | `TestKeybindingsManagerDoesNotEvictDefaultsWhenUserReusesKeys` | covered |
| 24 | still reports direct user binding conflicts without evicting defaults | `TestKeybindingsManagerReportsOnlyUserBindingConflicts` | covered |

## `keys.test.ts`

Pi cases: `59`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 47 | should match Ctrl+c when pressing Ctrl+С (Cyrillic) with base layout key | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 56 | should match Ctrl+d when pressing Ctrl+В (Cyrillic) with base layout key | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 64 | should match Ctrl+z when pressing Ctrl+Я (Cyrillic) with base layout key | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 72 | should match Ctrl+Shift+p with base layout key | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 81 | should still match direct codepoint when no base layout key | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 89 | should match super-modified Kitty bindings, including combined modifiers | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 104 | should match digit bindings via Kitty CSI-u | `TestKeysCSIuEventTypesDigitsAndCtrlAlt`, `TestKeysKittyLinefeedAndUnsupportedModifiers` | covered |
| 114 | should normalize Kitty keypad functional keys to logical digits, symbols, and navigation | `TestKeysCSIuEventTypesDigitsAndCtrlAlt`, `TestKeysKittyLinefeedAndUnsupportedModifiers` | covered |
| 137 | should handle shifted key in format | `TestKeysCSIuEventTypesDigitsAndCtrlAlt`, `TestKeysKittyLinefeedAndUnsupportedModifiers` | covered |
| 146 | should handle event type in format | `TestKeysCSIuEventTypesDigitsAndCtrlAlt`, `TestKeysKittyLinefeedAndUnsupportedModifiers` | covered |
| 155 | should handle full format with shifted key, base key, and event type | `TestKeysCSIuEventTypesDigitsAndCtrlAlt`, `TestKeysKittyLinefeedAndUnsupportedModifiers` | covered |
| 166 | should prefer codepoint for Latin letters even when base layout differs | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 175 | should prefer codepoint for symbol keys even when base layout differs | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 184 | should not match wrong key even with base layout | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 192 | should not match wrong modifiers even with base layout | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 202 | should match xterm modifyOtherKeys Ctrl+c | `TestKeysPiModifyOtherAndAltMatrices`, `TestDecodePrintableKeyIncludesXtermModifyOtherKeys` | covered |
| 208 | should match xterm modifyOtherKeys Ctrl+d | `TestKeysPiModifyOtherAndAltMatrices`, `TestDecodePrintableKeyIncludesXtermModifyOtherKeys` | covered |
| 214 | should match xterm modifyOtherKeys Ctrl+z | `TestKeysPiModifyOtherAndAltMatrices`, `TestDecodePrintableKeyIncludesXtermModifyOtherKeys` | covered |
| 220 | should match xterm modifyOtherKeys Enter variants | `TestKeysPiModifyOtherAndAltMatrices`, `TestDecodePrintableKeyIncludesXtermModifyOtherKeys` | covered |
| 230 | should match xterm modifyOtherKeys Tab variants | `TestKeysPiModifyOtherAndAltMatrices`, `TestDecodePrintableKeyIncludesXtermModifyOtherKeys` | covered |
| 240 | should match xterm modifyOtherKeys Backspace variants | `TestKeysPiModifyOtherAndAltMatrices`, `TestDecodePrintableKeyIncludesXtermModifyOtherKeys` | covered |
| 250 | should match xterm modifyOtherKeys Escape | `TestKeysPiModifyOtherAndAltMatrices`, `TestDecodePrintableKeyIncludesXtermModifyOtherKeys` | covered |
| 256 | should match xterm modifyOtherKeys Space variants | `TestKeysPiModifyOtherAndAltMatrices`, `TestDecodePrintableKeyIncludesXtermModifyOtherKeys` | covered |
| 264 | should match xterm modifyOtherKeys symbol combos | `TestKeysPiModifyOtherAndAltMatrices`, `TestDecodePrintableKeyIncludesXtermModifyOtherKeys` | covered |
| 270 | should match xterm modifyOtherKeys digit combos | `TestKeysPiModifyOtherAndAltMatrices`, `TestDecodePrintableKeyIncludesXtermModifyOtherKeys` | covered |
| 278 | should match xterm modifyOtherKeys shifted uppercase letters | `TestKeysPiModifyOtherAndAltMatrices`, `TestDecodePrintableKeyIncludesXtermModifyOtherKeys` | covered |
| 286 | should match Ctrl+Alt+letter via CSI-u when kitty inactive | `TestKeysCSIuEventTypesDigitsAndCtrlAlt`, `TestKeysKittyLinefeedAndUnsupportedModifiers` | covered |
| 292 | should match Ctrl+Alt+letter via xterm modifyOtherKeys | `TestKeysPiModifyOtherAndAltMatrices`, `TestDecodePrintableKeyIncludesXtermModifyOtherKeys` | covered |
| 300 | should match legacy Ctrl+c | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 306 | should match legacy Ctrl+d | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 312 | should match escape key | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 316 | should match legacy linefeed as enter | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 322 | should treat linefeed as shift+enter when kitty active | `TestKeysCSIuEventTypesDigitsAndCtrlAlt`, `TestKeysKittyLinefeedAndUnsupportedModifiers` | covered |
| 330 | should parse ctrl+space | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 336 | should match legacy Ctrl+symbol | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 351 | should match legacy Ctrl+Alt+symbol | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 369 | should treat raw 0x08 as plain backspace outside Windows Terminal | `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 382 | should treat raw 0x08 as ctrl+backspace in local Windows Terminal | `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 400 | should treat raw 0x08 as plain backspace in Windows Terminal over SSH | `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 418 | should parse legacy alt-prefixed sequences when kitty inactive | `TestKeysCSIuEventTypesDigitsAndCtrlAlt`, `TestKeysKittyLinefeedAndUnsupportedModifiers` | covered |
| 459 | should match arrow keys | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 466 | should match SS3 arrows and home/end | `TestKeysC1CSIAndSS3Equivalents` | covered |
| 475 | should match legacy function keys and clear | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 481 | should match alt+arrows | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 486 | should match rxvt modifier sequences | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 497 | should decode Kitty keypad functional keys to printable characters | `TestKeysCSIuEventTypesDigitsAndCtrlAlt`, `TestKeysKittyLinefeedAndUnsupportedModifiers` | covered |
| 512 | should decode printable xterm modifyOtherKeys sequences | `TestKeysPiModifyOtherAndAltMatrices`, `TestDecodePrintableKeyIncludesXtermModifyOtherKeys` | covered |
| 523 | should return Latin key name when base layout key is present | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 531 | should prefer codepoint for Latin letters when base layout differs | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 539 | should prefer codepoint for symbol keys when base layout differs | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 547 | should return key name from codepoint when no base layout | `TestKeysKittyAlternateLayoutAndSuperModifierMatrix` | covered |
| 554 | should parse shifted uppercase CSI-u letters as shift+letter | `TestKeysCSIuEventTypesDigitsAndCtrlAlt`, `TestKeysKittyLinefeedAndUnsupportedModifiers` | covered |
| 561 | should ignore Kitty CSI-u with unsupported modifiers | `TestKeysCSIuEventTypesDigitsAndCtrlAlt`, `TestKeysKittyLinefeedAndUnsupportedModifiers` | covered |
| 569 | should parse legacy Ctrl+letter | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 575 | should parse special keys | `TestKeyHelperMatchesPiKeyObject` | covered |
| 586 | should parse arrow keys | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 593 | should parse SS3 arrows and home/end | `TestKeysC1CSIAndSS3Equivalents` | covered |
| 602 | should parse legacy function and modifier sequences | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |
| 610 | should parse double bracket pageUp | `TestKeysLegacySequencesAndKittyAltGate`, `TestKeysLegacyControlAndWindowsTerminalBackspace` | covered |

## `markdown.test.ts`

Pi cases: `60`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 40 | should render simple nested list | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 66 | should render deeply nested list | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 87 | should render ordered nested list | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 107 | should render mixed ordered and unordered nested lists | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 127 | should render task list markers | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 135 | should maintain numbering when code blocks are not indented (LLM output) | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 172 | should indent wrapped unordered list lines | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 180 | should indent wrapped ordered list lines | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 188 | should indent wrapped ordered list lines with multi-digit markers | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 196 | should indent wrapped nested list lines | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 204 | should indent wrapped nested list lines under ordered parents | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 212 | should render and wrap blockquotes inside list items | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 220 | should render and wrap code blocks inside list items | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 235 | should render simple table | `TestMarkdownTable`, `TestMarkdownPiTableStructureAndWidthMatrix`, `TestMarkdownTableWrapsLongCellContent`, `TestMarkdownTablePiWidthBoundaries` | covered |
| 259 | should render row dividers between data rows | `TestMarkdownTable`, `TestMarkdownPiTableStructureAndWidthMatrix`, `TestMarkdownTableWrapsLongCellContent`, `TestMarkdownTablePiWidthBoundaries` | covered |
| 277 | should keep column width at least the longest word | `TestMarkdownTable`, `TestMarkdownPiTableStructureAndWidthMatrix`, `TestMarkdownTableWrapsLongCellContent`, `TestMarkdownTablePiWidthBoundaries` | covered |
| 305 | should render table with alignment | `TestMarkdownTable`, `TestMarkdownPiTableStructureAndWidthMatrix`, `TestMarkdownTableWrapsLongCellContent`, `TestMarkdownTablePiWidthBoundaries` | covered |
| 327 | should handle tables with varying column widths | `TestMarkdownTable`, `TestMarkdownPiTableStructureAndWidthMatrix`, `TestMarkdownTableWrapsLongCellContent`, `TestMarkdownTablePiWidthBoundaries` | covered |
| 348 | should wrap table cells when table exceeds available width | `TestMarkdownTable`, `TestMarkdownPiTableStructureAndWidthMatrix`, `TestMarkdownTableWrapsLongCellContent`, `TestMarkdownTablePiWidthBoundaries` | covered |
| 376 | should wrap long cell content to multiple lines | `TestMarkdownInlineFormattingAndLinks` | covered |
| 401 | should wrap long unbroken tokens inside table cells (not only at line start) | `TestMarkdownTable`, `TestMarkdownPiTableStructureAndWidthMatrix`, `TestMarkdownTableWrapsLongCellContent`, `TestMarkdownTablePiWidthBoundaries` | covered |
| 438 | should wrap styled inline code inside table cells without breaking borders | `TestMarkdownTable`, `TestMarkdownPiTableStructureAndWidthMatrix`, `TestMarkdownTableWrapsLongCellContent`, `TestMarkdownTablePiWidthBoundaries` | covered |
| 465 | should handle extremely narrow width gracefully | `TestMarkdownInlineFormattingAndLinks` | covered |
| 488 | should render table correctly when it fits naturally | `TestMarkdownTable`, `TestMarkdownPiTableStructureAndWidthMatrix`, `TestMarkdownTableWrapsLongCellContent`, `TestMarkdownTablePiWidthBoundaries` | covered |
| 514 | should respect paddingX when calculating table width | `TestMarkdownTable`, `TestMarkdownPiTableStructureAndWidthMatrix`, `TestMarkdownTableWrapsLongCellContent`, `TestMarkdownTablePiWidthBoundaries` | covered |
| 538 | should not add a trailing blank line when table is the last rendered block | `TestMarkdownTable`, `TestMarkdownPiTableStructureAndWidthMatrix`, `TestMarkdownTableWrapsLongCellContent`, `TestMarkdownTablePiWidthBoundaries` | covered |
| 560 | should render lists and tables together | `TestMarkdownCombinedListsAndTablesLikePi` | covered |
| 591 | should preserve gray italic styling after inline code | `TestMarkdownDefaultTextStyleRestoresAfterInlineCodeAndBold`, `TestMarkdownNestedInlineFormattingMatchesPiTokens` | covered |
| 619 | should preserve gray italic styling after bold text | `TestMarkdownDefaultTextStyleRestoresAfterInlineCodeAndBold`, `TestMarkdownNestedInlineFormattingMatchesPiTokens` | covered |
| 645 | should not leak styles into following lines when rendered in TUI | `TestMarkdownInlineFormattingAndLinks` | covered |
| 682 | should have only one blank line between code block and following paragraph | `TestMarkdownBlockquoteAndCodeFence`, `TestMarkdownIndentedCodeBlocksRenderLikePi`, `TestMarkdownPiBlockSpacingNormalization` | covered |
| 712 | should normalize paragraph and code block spacing to one blank line | `TestMarkdownBlockquoteAndCodeFence`, `TestMarkdownIndentedCodeBlocksRenderLikePi`, `TestMarkdownPiBlockSpacingNormalization` | covered |
| 742 | should not add a trailing blank line when code block is the last rendered block | `TestMarkdownBlockquoteAndCodeFence`, `TestMarkdownIndentedCodeBlocksRenderLikePi`, `TestMarkdownPiBlockSpacingNormalization` | covered |
| 760 | should have only one blank line between divider and following paragraph | `TestMarkdownPiBlockSpacingNormalization`, `TestMarkdownPiBlockSpacingNoTrailingBlank` | covered |
| 788 | should not add a trailing blank line when divider is the last rendered block | `TestMarkdownPiBlockSpacingNormalization`, `TestMarkdownPiBlockSpacingNoTrailingBlank` | covered |
| 802 | should have only one blank line between heading and following paragraph | `TestMarkdownHeadingStyleRestoresAfterInlineResets`, `TestMarkdownH1UnderlineDoesNotLeakIntoPadding` | covered |
| 828 | should not add a trailing blank line when heading is the last rendered block | `TestMarkdownHeadingStyleRestoresAfterInlineResets`, `TestMarkdownH1UnderlineDoesNotLeakIntoPadding` | covered |
| 842 | should have only one blank line between blockquote and following paragraph | `TestMarkdownBlockquoteLazyContinuationAndExplicitLines`, `TestMarkdownBlockquoteWrappedLinesKeepBorder`, `TestMarkdownBlockquoteRestoresQuoteStyleAfterInlineResets` | covered |
| 870 | should not add a trailing blank line when blockquote is the last rendered block | `TestMarkdownBlockquoteLazyContinuationAndExplicitLines`, `TestMarkdownBlockquoteWrappedLinesKeepBorder`, `TestMarkdownBlockquoteRestoresQuoteStyleAfterInlineResets` | covered |
| 884 | should apply consistent styling to all lines in lazy continuation blockquote | `TestMarkdownBlockquoteLazyContinuationAndExplicitLines`, `TestMarkdownBlockquoteWrappedLinesKeepBorder`, `TestMarkdownBlockquoteRestoresQuoteStyleAfterInlineResets` | covered |
| 919 | should apply consistent styling to explicit multiline blockquote | `TestMarkdownBlockquoteLazyContinuationAndExplicitLines`, `TestMarkdownBlockquoteWrappedLinesKeepBorder`, `TestMarkdownBlockquoteRestoresQuoteStyleAfterInlineResets` | covered |
| 949 | should render list content inside blockquotes | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 972 | should wrap long blockquote lines and add border to each wrapped line | `TestMarkdownBlockquoteLazyContinuationAndExplicitLines`, `TestMarkdownBlockquoteWrappedLinesKeepBorder`, `TestMarkdownBlockquoteRestoresQuoteStyleAfterInlineResets` | covered |
| 998 | should properly indent wrapped blockquote lines with styling | `TestMarkdownListsWrapAndIndent`, `TestMarkdownNestedOrderedAndMixedListsMatchPi`, `TestMarkdownListBlockquotesWrapWithPiIndentation`, `TestMarkdownListCodeBlockWrapsWithPiIndentation` | covered |
| 1029 | should render inline formatting inside blockquotes and reapply quote styling after | `TestMarkdownBlockquoteLazyContinuationAndExplicitLines`, `TestMarkdownBlockquoteWrappedLinesKeepBorder`, `TestMarkdownBlockquoteRestoresQuoteStyleAfterInlineResets` | covered |
| 1061 | should preserve heading styling after inline code | `TestMarkdownHeadingStyleRestoresAfterInlineResets`, `TestMarkdownH1UnderlineDoesNotLeakIntoPadding` | covered |
| 1090 | should preserve heading styling after inline code for h1 | `TestMarkdownHeadingStyleRestoresAfterInlineResets`, `TestMarkdownH1UnderlineDoesNotLeakIntoPadding` | covered |
| 1106 | should not leak h1 underline into padding when inline code is the last token | `TestMarkdownHeadingStyleRestoresAfterInlineResets`, `TestMarkdownH1UnderlineDoesNotLeakIntoPadding` | covered |
| 1126 | should preserve heading styling after bold text | `TestMarkdownHeadingStyleRestoresAfterInlineResets`, `TestMarkdownH1UnderlineDoesNotLeakIntoPadding` | covered |
| 1142 | should render ~~text~~ as strikethrough | `TestMarkdownStrikethroughUsesStrictDoubleTilde` | covered |
| 1154 | should keep ~text~ as plain text | `TestMarkdownStrikethroughUsesStrictDoubleTilde` | covered |
| 1171 | should not duplicate URL for autolinked emails | `TestMarkdownHyperlinksAndBareURLs`, `TestMarkdownLinkFallbackURLsWhenHyperlinksUnsupported`, `TestMarkdownMailtoAndEmailRendering` | covered |
| 1185 | should not duplicate URL for bare URLs | `TestMarkdownHyperlinksAndBareURLs`, `TestMarkdownLinkFallbackURLsWhenHyperlinksUnsupported`, `TestMarkdownMailtoAndEmailRendering` | covered |
| 1198 | should show URL in parentheses when hyperlinks are not supported | `TestMarkdownHyperlinksAndBareURLs`, `TestMarkdownLinkFallbackURLsWhenHyperlinksUnsupported`, `TestMarkdownMailtoAndEmailRendering` | covered |
| 1210 | should show mailto URL in parentheses when hyperlinks are not supported | `TestMarkdownHyperlinksAndBareURLs`, `TestMarkdownLinkFallbackURLsWhenHyperlinksUnsupported`, `TestMarkdownMailtoAndEmailRendering` | covered |
| 1222 | should emit OSC 8 hyperlink sequence when terminal supports hyperlinks | `TestMarkdownHyperlinksAndBareURLs`, `TestMarkdownLinkFallbackURLsWhenHyperlinksUnsupported`, `TestMarkdownMailtoAndEmailRendering` | covered |
| 1243 | should use OSC 8 for mailto links when terminal supports hyperlinks | `TestMarkdownHyperlinksAndBareURLs`, `TestMarkdownLinkFallbackURLsWhenHyperlinksUnsupported`, `TestMarkdownMailtoAndEmailRendering` | covered |
| 1257 | should use OSC 8 for bare URLs when terminal supports hyperlinks | `TestMarkdownHyperlinksAndBareURLs`, `TestMarkdownLinkFallbackURLsWhenHyperlinksUnsupported`, `TestMarkdownMailtoAndEmailRendering` | covered |
| 1274 | should render content with HTML-like tags as text | `TestMarkdownHTMLLikeTagsRemainVisible`, `TestMarkdownHTMLType1BlocksMatchCommonMark` | covered |
| 1295 | should render HTML tags in code blocks correctly | `TestMarkdownBlockquoteAndCodeFence`, `TestMarkdownIndentedCodeBlocksRenderLikePi`, `TestMarkdownPiBlockSpacingNormalization` | covered |

## `overlay-non-capturing.test.ts`

Pi cases: `24`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 49 | non-capturing overlay preserves focus on creation | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 67 | focus() transfers focus to the overlay | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 87 | unfocus() restores previous focus | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 108 | setHidden(false) on non-capturing overlay does not auto-focus | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 128 | hide() when overlay is not focused does not change focus | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 146 | hide() when focused restores focus correctly | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 166 | capturing overlay removed with non-capturing below restores focus to editor | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 188 | sub-overlay cleanup then hideOverlay restores focus and input to editor | `TestTUIHideOverlayPiTopmostNonCapturingAndHasOverlay` | covered |
| 218 | microtask-deferred sub-overlay pattern (showExtensionCustom simulation) restores focus | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 273 | handleInput redirection skips non-capturing overlays when focused overlay becomes invisible | `TestTUIOverlayPiInvisibleFocusedOverlayReroutesInput` | covered |
| 301 | hideOverlay() does not reassign focus when topmost overlay is non-capturing | `TestTUIHideOverlayPiTopmostNonCapturingAndHasOverlay` | covered |
| 322 | multiple capturing and non-capturing overlays restore focus through removals | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 350 | capturing overlay unfocus() on topmost capturing overlay falls back to preFocus | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 372 | focus() on hidden overlay is a no-op | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 392 | focus() after hide() is a no-op | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 412 | unfocus() when overlay does not have focus is a no-op | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 431 | unfocus() with null preFocus clears focus and does not route input back to overlay | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 453 | toggle focus between non-capturing overlays then unfocus returns to editor | `TestTUIOverlayPiNonCapturingFocusManagement`, `TestTUIOverlayPiNonCapturingFocusRestorationMatrix` | covered |
| 480 | focus() on already-focused overlay bumps visual order | `TestTUIOverlayPiFocusControlsVisualOrder`, `TestTUIOverlayPiFocusOrderMatrix` | covered |
| 503 | default rendering order for overlapping overlays follows creation order | `TestTUIOverlayPiFocusControlsVisualOrder`, `TestTUIOverlayPiFocusOrderMatrix` | covered |
| 518 | focus() on lower overlay renders it on top | `TestTUIOverlayPiFocusControlsVisualOrder`, `TestTUIOverlayPiFocusOrderMatrix` | covered |
| 536 | focusing middle overlay places it on top while preserving others relative order | `TestTUIOverlayPiFocusControlsVisualOrder`, `TestTUIOverlayPiFocusOrderMatrix` | covered |
| 561 | capturing overlay hidden and shown again renders on top after unhide | `TestTUIOverlayPiFocusControlsVisualOrder`, `TestTUIOverlayPiFocusOrderMatrix` | covered |
| 583 | unfocus() does not change visual order until another overlay is focused | `TestTUIOverlayPiFocusControlsVisualOrder`, `TestTUIOverlayPiFocusOrderMatrix` | covered |

## `overlay-options.test.ts`

Pi cases: `24`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 37 | should truncate overlay lines that exceed declared width | `TestTUIOverlayPiOptionsANSIAndWideOverflowMatrix`, `TestTUIOverlayCompositePreservesStyledSuffix` | covered |
| 58 | should handle overlay with complex ANSI sequences without crashing | `TestTUIOverlayPiOptionsANSIAndWideOverflowMatrix`, `TestTUIOverlayCompositePreservesStyledSuffix` | covered |
| 79 | should handle overlay composited on styled base content | `TestTUIOverlayPiOptionsANSIAndWideOverflowMatrix`, `TestTUIOverlayCompositePreservesStyledSuffix` | covered |
| 106 | should handle wide characters at overlay boundary | `TestTUIOverlayPiOptionsANSIAndWideOverflowMatrix`, `TestTUIOverlayCompositePreservesStyledSuffix` | covered |
| 124 | should handle overlay positioned at terminal edge | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 142 | should handle overlay on base content with OSC sequences | `TestTUIOverlayPiOptionsANSIAndWideOverflowMatrix`, `TestTUIOverlayCompositePreservesStyledSuffix` | covered |
| 171 | should render overlay at percentage of terminal width | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 185 | should respect minWidth when widthPercent results in smaller width | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 201 | should position overlay at top-left | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 216 | should position overlay at bottom-right | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 234 | should position overlay at top-center | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 256 | should clamp negative margins to zero | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 277 | should respect margin as number | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 298 | should respect margin object | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 321 | should apply offsetX and offsetY from anchor position | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 340 | should position with rowPercent and colPercent | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 365 | rowPercent 0 should position at top | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 380 | rowPercent 100 should position at bottom | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 397 | should truncate overlay to maxHeight | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 417 | should truncate overlay to maxHeightPercent | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 438 | row and col should override anchor | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 458 | should render multiple overlays with later ones on top | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 483 | should handle overlays at different positions without interference | `TestTUIOverlayPiOptionsLayout`, `TestTUIOverlayPiDecimalPercentSizeValues`, `TestTUIOverlayPiMarginMaxHeightAndWidthClipping`, `TestTUIOverlayPiOptionsPositioningMatrix` | covered |
| 507 | should properly hide overlays in stack order | `TestTUIOverlayRendersAndRestoresFocus`, `TestTUIOverlayLifecycleUsesDiffRenderWithoutClearing` | covered |

## `overlay-short-content.test.ts`

Pi cases: `1`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 22 | should render overlay when content is shorter than terminal height | `TestTUIOverlayRendersWhenContentShorterThanTerminal` | covered |

## `regression-regional-indicator-width.test.ts`

Pi cases: `5`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 6 | treats partial flag grapheme as full-width to avoid streaming render drift | `TestVisibleWidthPiGraphemeClusters`, `TestVisibleWidthPiEmojiPresentationClusters` | covered |
| 18 | wraps intermediate partial-flag list line before overflow | `TestWrapTextWithANSIPartialFlagBeforeOverflow` | covered |
| 28 | treats all regional-indicator singleton graphemes as width 2 | `TestVisibleWidthPiGraphemeClusters`, `TestVisibleWidthPiEmojiPresentationClusters` | covered |
| 39 | keeps full flag pairs at width 2 | `TestVisibleWidthPiGraphemeClusters`, `TestVisibleWidthPiEmojiPresentationClusters` | covered |
| 46 | keeps common streaming emoji intermediates at stable width | `TestVisibleWidthPiGraphemeClusters`, `TestVisibleWidthPiEmojiPresentationClusters` | covered |

## `select-list.test.ts`

Pi cases: `5`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 21 | normalizes multiline descriptions to single line | `TestSelectListNormalizesMultilineDescriptions` | covered |
| 38 | keeps descriptions aligned when the primary text is truncated | `TestSelectListDescriptionAlignmentWithTruncatedPrimary` | covered |
| 54 | uses the configured minimum primary column width | `TestSelectListPrimaryColumnMinWidth` | covered |
| 70 | uses the configured maximum primary column width | `TestSelectListPrimaryColumnMaxWidth` | covered |
| 90 | allows overriding primary truncation while preserving description alignment | `TestSelectListTruncatePrimaryOverridePreservesDescriptionAlignment` | covered |

## `stdin-buffer.test.ts`

Pi cases: `54`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 37 | should pass through regular characters immediately | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 42 | should pass through multiple regular characters | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 47 | should handle unicode characters | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 54 | should pass through complete mouse SGR sequences | `TestStdinBufferPiRegularAndCompleteSequences`, `TestStdinBufferPiMixedKittyMouseAndPasteMatrix` | covered |
| 60 | should pass through complete arrow key sequences | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 66 | should pass through complete function key sequences | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 72 | should pass through meta key sequences | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 78 | should pass through SS3 sequences | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 86 | should buffer incomplete mouse SGR sequence | `TestStdinBufferPiRegularAndCompleteSequences`, `TestStdinBufferPiMixedKittyMouseAndPasteMatrix` | covered |
| 100 | should buffer incomplete CSI sequence | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |
| 111 | should buffer split across many chunks | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |
| 127 | should flush incomplete sequence after timeout | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |
| 139 | should handle characters followed by escape sequence | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 144 | should handle escape sequence followed by characters | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 149 | should handle multiple complete sequences | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 154 | should handle partial sequence with preceding characters | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |
| 165 | should handle Kitty CSI u press events | `TestStdinBufferPiKittySequencesAndPrintableDedupe`, `TestStdinBufferPiMouseAndKittyDedup` | covered |
| 171 | should handle Kitty CSI u release events | `TestStdinBufferPiKittySequencesAndPrintableDedupe`, `TestStdinBufferPiMouseAndKittyDedup` | covered |
| 177 | should handle batched Kitty press and release | `TestStdinBufferPiKittySequencesAndPrintableDedupe`, `TestStdinBufferPiMouseAndKittyDedup` | covered |
| 183 | should handle multiple batched Kitty events | `TestStdinBufferPiKittySequencesAndPrintableDedupe`, `TestStdinBufferPiMouseAndKittyDedup` | covered |
| 189 | should handle Kitty arrow keys with event type | `TestStdinBufferPiKittySequencesAndPrintableDedupe`, `TestStdinBufferPiMouseAndKittyDedup` | covered |
| 195 | should handle Kitty functional keys with event type | `TestStdinBufferPiKittySequencesAndPrintableDedupe`, `TestStdinBufferPiMouseAndKittyDedup` | covered |
| 201 | should split ESC+ESC+CSI into standalone ESC and the CSI sequence (WezTerm Escape key regression) | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 210 | should split ESC+ESC+CSI with no modifier (no num_lock) | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 215 | should still emit ESC+ESC as a single sequence when not followed by a new escape | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 221 | should handle plain characters mixed with Kitty sequences | `TestStdinBufferPiKittySequencesAndPrintableDedupe`, `TestStdinBufferPiMouseAndKittyDedup` | covered |
| 227 | should drop raw duplicate character after matching Kitty printable sequence | `TestStdinBufferPiKittySequencesAndPrintableDedupe`, `TestStdinBufferPiMouseAndKittyDedup` | covered |
| 232 | should drop raw duplicate character after matching Kitty printable sequence across chunks | `TestStdinBufferPiKittySequencesAndPrintableDedupe`, `TestStdinBufferPiMouseAndKittyDedup` | covered |
| 238 | should keep non-matching plain character after Kitty printable sequence | `TestStdinBufferPiKittySequencesAndPrintableDedupe`, `TestStdinBufferPiMouseAndKittyDedup` | covered |
| 243 | should keep raw character after modified Kitty printable sequence | `TestStdinBufferPiKittySequencesAndPrintableDedupe`, `TestStdinBufferPiMouseAndKittyDedup` | covered |
| 248 | should handle rapid typing simulation with Kitty protocol | `TestStdinBufferPiKittySequencesAndPrintableDedupe`, `TestStdinBufferPiMouseAndKittyDedup` | covered |
| 256 | should handle mouse press event | `TestStdinBufferPiRegularAndCompleteSequences`, `TestStdinBufferPiMixedKittyMouseAndPasteMatrix` | covered |
| 261 | should handle mouse release event | `TestStdinBufferPiRegularAndCompleteSequences`, `TestStdinBufferPiMixedKittyMouseAndPasteMatrix` | covered |
| 266 | should handle mouse move event | `TestStdinBufferPiRegularAndCompleteSequences`, `TestStdinBufferPiMixedKittyMouseAndPasteMatrix` | covered |
| 271 | should handle split mouse events | `TestStdinBufferPiRegularAndCompleteSequences`, `TestStdinBufferPiMixedKittyMouseAndPasteMatrix` | covered |
| 279 | should handle multiple mouse events | `TestStdinBufferPiRegularAndCompleteSequences`, `TestStdinBufferPiMixedKittyMouseAndPasteMatrix` | covered |
| 284 | should handle old-style mouse sequence (ESC[M + 3 bytes) | `TestStdinBufferPiRegularAndCompleteSequences`, `TestStdinBufferPiMixedKittyMouseAndPasteMatrix` | covered |
| 289 | should buffer incomplete old-style mouse sequence | `TestStdinBufferPiRegularAndCompleteSequences`, `TestStdinBufferPiMixedKittyMouseAndPasteMatrix` | covered |
| 302 | should handle empty input | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |
| 308 | should handle lone escape character with timeout | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |
| 317 | should handle lone escape character with explicit flush | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |
| 325 | should handle buffer input | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |
| 330 | should handle very long sequences | `TestStdinBufferPiRegularAndCompleteSequences` | covered |
| 338 | should flush incomplete sequences | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |
| 345 | should return empty array if nothing to flush | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |
| 350 | should emit flushed data via timeout | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |
| 362 | should clear buffered content without emitting | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |
| 391 | should emit paste event for complete bracketed paste | `TestStdinBufferPiPasteClearAndDestroy`, `TestStdinBufferPasteClearsKittyPrintableDedupe` | covered |
| 402 | should handle paste arriving in chunks | `TestStdinBufferPiPasteClearAndDestroy`, `TestStdinBufferPasteClearsKittyPrintableDedupe` | covered |
| 414 | should handle paste with input before and after | `TestStdinBufferPiPasteClearAndDestroy`, `TestStdinBufferPasteClearsKittyPrintableDedupe` | covered |
| 423 | should handle paste with newlines | `TestStdinBufferPiPasteClearAndDestroy`, `TestStdinBufferPasteClearsKittyPrintableDedupe` | covered |
| 430 | should handle paste with unicode | `TestStdinBufferPiPasteClearAndDestroy`, `TestStdinBufferPasteClearsKittyPrintableDedupe` | covered |
| 439 | should clear buffer on destroy | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |
| 447 | should clear pending timeouts on destroy | `TestStdinBufferPiPartialAndMixedSequences`, `TestStdinBufferTimeoutFlushesIncompleteSequence`, `TestStdinBufferExplicitFlushPiEdgeCases`, `TestStdinBufferClearAndDestroyDiscardPendingInput` | covered |

## `terminal-image.test.ts`

Pi cases: `43`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 55 | should detect iTerm2 image escape sequence at start of line | `TestIsImageLinePiCoverageMatrix` | covered |
| 61 | should detect iTerm2 image escape sequence with text before it | `TestIsImageLinePiCoverageMatrix` | covered |
| 67 | should detect iTerm2 image escape sequence in middle of long line | `TestIsImageLinePiCoverageMatrix` | covered |
| 74 | should detect iTerm2 image escape sequence at end of line | `TestIsImageLinePiCoverageMatrix` | covered |
| 79 | should detect minimal iTerm2 image escape sequence | `TestIsImageLinePiCoverageMatrix` | covered |
| 86 | should detect Kitty image escape sequence at start of line | `TestIsImageLinePiCoverageMatrix` | covered |
| 92 | should detect Kitty image escape sequence with text before it | `TestIsImageLinePiCoverageMatrix` | covered |
| 98 | should detect Kitty image escape sequence with padding | `TestIsImageLinePiCoverageMatrix` | covered |
| 106 | should detect image sequences in very long lines (304k+ chars) | `TestIsImageLinePiCoverageMatrix` | covered |
| 123 | should detect image sequences when terminal doesn't support images | `TestIsImageLinePiCoverageMatrix` | covered |
| 130 | should detect image sequences with ANSI codes before them | `TestIsImageLinePiCoverageMatrix` | covered |
| 136 | should detect image sequences with ANSI codes after them | `TestIsImageLinePiCoverageMatrix` | covered |
| 143 | should not detect images in plain text lines | `TestIsImageLinePiCoverageMatrix` | covered |
| 148 | should not detect images in lines with only ANSI codes | `TestIsImageLinePiCoverageMatrix` | covered |
| 153 | should not detect images in lines with cursor movement codes | `TestRenderImageKittyCursorMovementOption`, `TestEncodeKittyDeleteAndHyperlink` | covered |
| 158 | should not detect images in lines with partial iTerm2 sequences | `TestIsImageLinePiCoverageMatrix` | covered |
| 164 | should not detect images in lines with partial Kitty sequences | `TestIsImageLinePiCoverageMatrix` | covered |
| 170 | should not detect images in empty lines | `TestIsImageLinePiCoverageMatrix` | covered |
| 174 | should not detect images in lines with newlines only | `TestIsImageLinePiCoverageMatrix` | covered |
| 181 | should detect images when line has both Kitty and iTerm2 sequences | `TestIsImageLinePiCoverageMatrix` | covered |
| 186 | should detect image in line with multiple text and image segments | `TestIsImageLinePiCoverageMatrix` | covered |
| 191 | should not falsely detect image in line with file path containing keywords | `TestIsImageLinePiCoverageMatrix` | covered |
| 200 | defaults to hyperlinks: false for unknown terminals | `TestHyperlinkPiOSC8ExactRendering` | covered |
| 208 | forces hyperlinks: false under tmux even if outer terminal supports OSC 8 | `TestHyperlinkPiOSC8ExactRendering` | covered |
| 216 | forces hyperlinks: false when TERM starts with 'tmux' | `TestHyperlinkPiOSC8ExactRendering` | covered |
| 224 | forces hyperlinks: false when TERM starts with 'screen' | `TestHyperlinkPiOSC8ExactRendering` | covered |
| 232 | enables hyperlinks for Ghostty | `TestHyperlinkPiOSC8ExactRendering` | covered |
| 239 | does not disable Ghostty images solely because cmux is present | `TestDetectCapabilitiesFromEnvironment` | covered |
| 247 | enables hyperlinks for Kitty | `TestHyperlinkPiOSC8ExactRendering` | covered |
| 254 | enables hyperlinks for WezTerm | `TestHyperlinkPiOSC8ExactRendering` | covered |
| 261 | enables hyperlinks for iTerm2 | `TestHyperlinkPiOSC8ExactRendering` | covered |
| 268 | enables hyperlinks for VSCode | `TestHyperlinkPiOSC8ExactRendering` | covered |
| 277 | can request no terminal-side cursor movement | `TestRenderImageKittyCursorMovementOption`, `TestEncodeKittyDeleteAndHyperlink` | covered |
| 282 | suppresses Kitty replies for delete commands | `TestRenderImageKittyCursorMovementOption`, `TestEncodeKittyDeleteAndHyperlink` | covered |
| 287 | preserves renderImage's default terminal-side cursor movement | `TestRenderImageKittyCursorMovementOption`, `TestEncodeKittyDeleteAndHyperlink` | covered |
| 301 | can opt renderImage into no terminal-side cursor movement | `TestRenderImageKittyCursorMovementOption`, `TestEncodeKittyDeleteAndHyperlink` | covered |
| 315 | honors maxHeightCells by reducing rendered width | `TestImageDimensionsAndCellSizing` | covered |
| 329 | caps Image component height to a square pixel box by default | `TestImageDimensionsAndCellSizing` | covered |
| 349 | places image sequence on first line with empty padding rows | `TestImageComponentRendersKittySequenceAndPaddingRows`, `TestImageComponentRendersITerm2PlacementOnLastRow` | covered |
| 376 | wraps text in OSC 8 open and close sequences | `TestHyperlinkPiOSC8ExactRendering` | covered |
| 381 | preserves ANSI styling inside the hyperlink | `TestHyperlinkPiOSC8ExactRendering` | covered |
| 389 | works with empty text | `TestHyperlinkPiOSC8ExactRendering` | covered |
| 394 | works with file:// URIs | `TestHyperlinkPiOSC8ExactRendering` | covered |

## `terminal.test.ts`

Pi cases: `1`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 6 | falls back to COLUMNS and LINES before default dimensions | `TestProcessTerminalDimensionsFromEnvironment` | covered |

## `truncate-to-width.test.ts`

Pi cases: `11`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 6 | keeps output within width for very large unicode input | `TestTruncateToWidthPiLargeMalformedAndContiguousPrefix` | covered |
| 14 | preserves ANSI styling for kept text and resets before and after ellipsis | `TestTruncateToWidthPreservesAnsiAndPads` | covered |
| 23 | handles malformed ANSI escape prefixes without hanging | `TestTruncateToWidthPiLargeMalformedAndContiguousPrefix` | covered |
| 30 | clips wide ellipsis safely and brackets it with resets | `TestTruncateToWidthPiEdgeCases` | covered |
| 36 | returns the original text when it already fits even if ellipsis is too wide | `TestTruncateToWidthPiEdgeCases` | covered |
| 41 | pads truncated output to requested width | `TestTruncateToWidthPreservesAnsiAndPads` | covered |
| 46 | adds a trailing reset when truncating without an ellipsis | `TestTruncateToWidthPiEdgeCases` | covered |
| 52 | keeps a contiguous prefix instead of skipping a wide grapheme and resuming later | `TestTruncateToWidthPiLargeMalformedAndContiguousPrefix` | covered |
| 59 | counts tabs inline and skips ANSI inline | `TestTruncateToWidthPreservesAnsiAndPads` | covered |
| 63 | keeps Thai and Lao AM clusters at their normal cell width | `TestTruncateToWidthPiEdgeCases` | covered |
| 70 | normalizes Thai and Lao AM vowels only for terminal output | `TestNormalizeTerminalOutput` | covered |

## `truncated-text.test.ts`

Pi cases: `9`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 11 | pads output lines to exactly match width | `TestTruncatedTextPadsAndTruncates`, `TestTruncatedTextVerticalPaddingANSIAndNewlines`, `TestTruncatedTextPiComponentMatrix` | covered |
| 23 | pads output with vertical padding lines to width | `TestTruncatedTextPadsAndTruncates`, `TestTruncatedTextVerticalPaddingANSIAndNewlines`, `TestTruncatedTextPiComponentMatrix` | covered |
| 36 | truncates long text and pads to width | `TestTruncatedTextPadsAndTruncates`, `TestTruncatedTextVerticalPaddingANSIAndNewlines`, `TestTruncatedTextPiComponentMatrix` | covered |
| 51 | preserves ANSI codes in output and pads correctly | `TestTruncatedTextPadsAndTruncates`, `TestTruncatedTextVerticalPaddingANSIAndNewlines`, `TestTruncatedTextPiComponentMatrix` | covered |
| 65 | truncates styled text and adds reset code before ellipsis | `TestTruncatedTextPadsAndTruncates`, `TestTruncatedTextVerticalPaddingANSIAndNewlines`, `TestTruncatedTextPiComponentMatrix` | covered |
| 79 | handles text that fits exactly | `TestTruncatedTextPadsAndTruncates`, `TestTruncatedTextVerticalPaddingANSIAndNewlines`, `TestTruncatedTextPiComponentMatrix` | covered |
| 93 | handles empty text | `TestTruncatedTextPadsAndTruncates`, `TestTruncatedTextVerticalPaddingANSIAndNewlines`, `TestTruncatedTextPiComponentMatrix` | covered |
| 101 | stops at newline and only shows first line | `TestTruncatedTextPadsAndTruncates`, `TestTruncatedTextVerticalPaddingANSIAndNewlines`, `TestTruncatedTextPiComponentMatrix` | covered |
| 116 | truncates first line even with newlines in text | `TestTruncatedTextPadsAndTruncates`, `TestTruncatedTextVerticalPaddingANSIAndNewlines`, `TestTruncatedTextPiComponentMatrix` | covered |

## `tui-cell-size-input.test.ts`

Pi cases: `2`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 45 | forwards bare escape even when a cell size query was sent at startup | `TestTUIConsumesCellSizeResponsesAndForwardsEscape` | covered |
| 61 | consumes cell size responses and still forwards later user input | `TestTUIConsumesCellSizeResponsesAndForwardsEscape`, `TestTUICellSizeResponseInvalidatesAndRerenders` | covered |

## `tui-overlay-style-leak.test.ts`

Pi cases: `2`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 44 | should not leak styles when a trailing reset sits beyond the last visible column (no overlay) | `TestVirtualTerminalTracksItalicResetAtLineBoundary` | covered |
| 57 | should not leak styles when overlay slicing drops trailing SGR resets | `TestTUIOverlayLinesKeepResetAfterSlicing`, `TestVirtualTerminalTracksItalicResetAfterOverlaySlicing` | covered |

## `tui-render.test.ts`

Pi cases: `19`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 68 | deletes changed image ids before drawing moved placements | `TestTUIDeletesKittyImagesBeforeRedraw`, `TestTUIDeletesChangedKittyImageBeforeMovedPlacement`, `TestTUIRedrawsKittyImageLineWhenReservedRowChanges` | covered |
| 95 | redraws image lines when an earlier reserved image row changes | `TestTUIDeletesKittyImagesBeforeRedraw`, `TestTUIDeletesChangedKittyImageBeforeMovedPlacement`, `TestTUIRedrawsKittyImageLineWhenReservedRowChanges` | covered |
| 122 | deletes previously rendered image ids during full redraws | `TestTUIDeletesKittyImagesBeforeRedraw`, `TestTUIDeletesChangedKittyImageBeforeMovedPlacement`, `TestTUIRedrawsKittyImageLineWhenReservedRowChanges` | covered |
| 149 | triggers full re-render when terminal height changes | `TestTUIResizeFullRedrawPolicy`, `TestTUIResizeSkipsTermuxHeightFullRedraw` | covered |
| 176 | skips full re-render on height changes in Termux | `TestTUIResizeFullRedrawPolicy`, `TestTUIResizeSkipsTermuxHeightFullRedraw` | covered |
| 205 | triggers full re-render when terminal width changes | `TestTUIResizeFullRedrawPolicy`, `TestTUIResizeSkipsTermuxHeightFullRedraw` | covered |
| 229 | clears empty rows when content shrinks significantly | `TestVirtualTerminalTUIShrinkClearsStaleRows`, `TestVirtualTerminalTUIShrinkResetsViewportTop`, `TestVirtualTerminalTUIViewportMovedUpForcesFullRedrawWithoutClearOnShrink`, `TestVirtualTerminalTUIAppendAfterViewportResetUsesDiff`, `TestVirtualTerminalTUIClearsTransientComponentHighWater` | covered |
| 261 | handles shrink to single line | `TestVirtualTerminalTUIShrinkClearsStaleRows`, `TestVirtualTerminalTUIShrinkResetsViewportTop`, `TestVirtualTerminalTUIViewportMovedUpForcesFullRedrawWithoutClearOnShrink`, `TestVirtualTerminalTUIAppendAfterViewportResetUsesDiff`, `TestVirtualTerminalTUIClearsTransientComponentHighWater` | covered |
| 284 | handles shrink to empty | `TestVirtualTerminalTUIShrinkClearsStaleRows`, `TestVirtualTerminalTUIShrinkResetsViewportTop`, `TestVirtualTerminalTUIViewportMovedUpForcesFullRedrawWithoutClearOnShrink`, `TestVirtualTerminalTUIAppendAfterViewportResetUsesDiff`, `TestVirtualTerminalTUIClearsTransientComponentHighWater` | covered |
| 310 | tracks cursor correctly when content shrinks with unchanged remaining lines | `TestVirtualTerminalTUIShrinkClearsStaleRows`, `TestVirtualTerminalTUIShrinkResetsViewportTop`, `TestVirtualTerminalTUIViewportMovedUpForcesFullRedrawWithoutClearOnShrink`, `TestVirtualTerminalTUIAppendAfterViewportResetUsesDiff`, `TestVirtualTerminalTUIClearsTransientComponentHighWater` | covered |
| 339 | renders correctly when only a middle line changes (spinner case) | `TestVirtualTerminalTUIDifferentialSpinnerPreservesRows`, `TestVirtualTerminalTUIDifferentialPiChangedLineMatrix`, `TestTUIDiffRenderingOnlyWritesChangedRangeLikePi` | covered |
| 366 | resets styles after each rendered line | `TestTUIUsesSynchronizedFullAndDiffRendering` | covered |
| 380 | renders correctly when first line changes but rest stays same | `TestVirtualTerminalTUIDifferentialSpinnerPreservesRows`, `TestVirtualTerminalTUIDifferentialPiChangedLineMatrix`, `TestTUIDiffRenderingOnlyWritesChangedRangeLikePi` | covered |
| 404 | renders correctly when last line changes but rest stays same | `TestVirtualTerminalTUIDifferentialSpinnerPreservesRows`, `TestVirtualTerminalTUIDifferentialPiChangedLineMatrix`, `TestTUIDiffRenderingOnlyWritesChangedRangeLikePi` | covered |
| 428 | renders correctly when multiple non-adjacent lines change | `TestVirtualTerminalTUIDifferentialSpinnerPreservesRows`, `TestVirtualTerminalTUIDifferentialPiChangedLineMatrix`, `TestTUIDiffRenderingOnlyWritesChangedRangeLikePi` | covered |
| 453 | handles transition from content to empty and back to content | `TestVirtualTerminalTUIShrinkClearsStaleRows`, `TestVirtualTerminalTUIShrinkResetsViewportTop`, `TestVirtualTerminalTUIViewportMovedUpForcesFullRedrawWithoutClearOnShrink`, `TestVirtualTerminalTUIAppendAfterViewportResetUsesDiff`, `TestVirtualTerminalTUIClearsTransientComponentHighWater` | covered |
| 484 | full re-renders when deleted lines move the viewport upward | `TestVirtualTerminalTUIShrinkClearsStaleRows`, `TestVirtualTerminalTUIShrinkResetsViewportTop`, `TestVirtualTerminalTUIViewportMovedUpForcesFullRedrawWithoutClearOnShrink`, `TestVirtualTerminalTUIAppendAfterViewportResetUsesDiff`, `TestVirtualTerminalTUIClearsTransientComponentHighWater` | covered |
| 506 | appends after a shrink without another full redraw once the viewport is reset | `TestVirtualTerminalTUIShrinkClearsStaleRows`, `TestVirtualTerminalTUIShrinkResetsViewportTop`, `TestVirtualTerminalTUIViewportMovedUpForcesFullRedrawWithoutClearOnShrink`, `TestVirtualTerminalTUIAppendAfterViewportResetUsesDiff`, `TestVirtualTerminalTUIClearsTransientComponentHighWater` | covered |
| 535 | clears stale content when maxLinesRendered was inflated by a transient component | `TestVirtualTerminalTUIShrinkClearsStaleRows`, `TestVirtualTerminalTUIShrinkResetsViewportTop`, `TestVirtualTerminalTUIViewportMovedUpForcesFullRedrawWithoutClearOnShrink`, `TestVirtualTerminalTUIAppendAfterViewportResetUsesDiff`, `TestVirtualTerminalTUIClearsTransientComponentHighWater` | covered |

## `wrap-ansi.test.ts`

Pi cases: `15`

| Pi line | Pi test case | Gi coverage | Status |
|---:|---|---|---|
| 7 | should not apply underline style before the styled text | `TestWrapTextWithANSIPiUnderlineAndBackgroundEdgeCases`, `TestWrapTextWithANSIClosesTransientStylesAtLineBreaks` | covered |
| 23 | should not have whitespace before underline reset code | `TestWrapTextWithANSIPiUnderlineAndBackgroundEdgeCases`, `TestWrapTextWithANSIClosesTransientStylesAtLineBreaks` | covered |
| 33 | should not bleed underline to padding - each line should end with reset for underline only | `TestWrapTextWithANSIPiUnderlineAndBackgroundEdgeCases`, `TestWrapTextWithANSIClosesTransientStylesAtLineBreaks` | covered |
| 55 | should preserve background color across wrapped lines without full reset | `TestWrapTextWithANSIPiUnderlineAndBackgroundEdgeCases`, `TestWrapTextWithANSIClosesTransientStylesAtLineBreaks` | covered |
| 73 | should reset underline but preserve background when wrapping underlined text inside background | `TestWrapTextWithANSIPiUnderlineAndBackgroundEdgeCases`, `TestWrapTextWithANSIClosesTransientStylesAtLineBreaks` | covered |
| 104 | should wrap plain text correctly | `TestWrapTextWithANSI` | covered |
| 114 | should ignore OSC 133 semantic markers in visible width | `TestWrapTextWithANSIPiOSCVisibleWidthMarkers` | covered |
| 119 | should ignore OSC sequences terminated with ST in visible width | `TestWrapTextWithANSIPiOSCVisibleWidthMarkers` | covered |
| 124 | should treat isolated regional indicators as width 2 | `TestVisibleWidthPiGraphemeClusters` | covered |
| 129 | should truncate trailing whitespace that exceeds width | `TestWrapTextWithANSI` | covered |
| 134 | should preserve color codes across wraps | `TestWrapTextWithANSI` | covered |
| 155 | re-emits OSC 8 open at the start of continuation lines | `TestWrapTextWithANSIOSC8HyperlinksCloseAndReopen` | covered |
| 177 | closes OSC 8 before each line break | `TestWrapTextWithANSIOSC8HyperlinksCloseAndReopen` | covered |
| 194 | preserves BEL terminators when wrapping OAuth-style hyperlinks | `TestWrapTextWithANSIOSC8HyperlinksCloseAndReopen` | covered |
| 209 | does not emit OSC 8 sequences on lines that are outside the hyperlink | `TestWrapTextWithANSIOSC8HyperlinksCloseAndReopen` | covered |

