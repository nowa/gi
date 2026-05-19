package gitui

// KillRing stores Emacs-style kill/yank entries.
//
// Consecutive kills can be accumulated into the newest entry. Backward
// deletions prepend killed text while forward deletions append it.
type KillRing struct {
	ring []string
}

type KillRingPushOptions struct {
	Prepend    bool
	Accumulate bool
}

func NewKillRing() *KillRing {
	return &KillRing{}
}

func (k *KillRing) Push(text string, opts KillRingPushOptions) {
	if text == "" {
		return
	}
	if opts.Accumulate && len(k.ring) > 0 {
		last := k.ring[len(k.ring)-1]
		if opts.Prepend {
			k.ring[len(k.ring)-1] = text + last
		} else {
			k.ring[len(k.ring)-1] = last + text
		}
		return
	}
	k.ring = append(k.ring, text)
}

func (k *KillRing) Peek() (string, bool) {
	if len(k.ring) == 0 {
		return "", false
	}
	return k.ring[len(k.ring)-1], true
}

func (k *KillRing) Rotate() {
	if len(k.ring) <= 1 {
		return
	}
	last := k.ring[len(k.ring)-1]
	copy(k.ring[1:], k.ring[:len(k.ring)-1])
	k.ring[0] = last
}

func (k *KillRing) Len() int {
	return len(k.ring)
}

func (k *KillRing) Length() int {
	return k.Len()
}

// UndoStack stores state snapshots. When constructed with a clone function,
// Push stores the cloned state so later caller mutations do not alter the
// snapshot. Without a clone function, S is copied by value.
type UndoStack[S any] struct {
	stack []S
	clone func(S) S
}

func NewUndoStack[S any](clone ...func(S) S) *UndoStack[S] {
	var cloneFn func(S) S
	if len(clone) > 0 {
		cloneFn = clone[0]
	}
	return &UndoStack[S]{clone: cloneFn}
}

func (u *UndoStack[S]) Push(state S) {
	if u.clone != nil {
		state = u.clone(state)
	}
	u.stack = append(u.stack, state)
}

func (u *UndoStack[S]) Pop() (S, bool) {
	if len(u.stack) == 0 {
		var zero S
		return zero, false
	}
	last := u.stack[len(u.stack)-1]
	u.stack = u.stack[:len(u.stack)-1]
	return last, true
}

func (u *UndoStack[S]) Clear() {
	u.stack = nil
}

func (u *UndoStack[S]) Len() int {
	return len(u.stack)
}

func (u *UndoStack[S]) Length() int {
	return u.Len()
}
