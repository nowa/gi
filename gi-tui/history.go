package gitui

import internalhistory "github.com/nowa/gi/gi-tui/internal/history"

type KillRing = internalhistory.KillRing
type KillRingPushOptions = internalhistory.KillRingPushOptions

func NewKillRing() *KillRing {
	return internalhistory.NewKillRing()
}

type UndoStack[S any] = internalhistory.UndoStack[S]

func NewUndoStack[S any](clone ...func(S) S) *UndoStack[S] {
	return internalhistory.NewUndoStack(clone...)
}
