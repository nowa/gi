package gitui

import "github.com/nowa/gi/gi-tui/internal/vtemu"

type VirtualTerminal = vtemu.VirtualTerminal
type VirtualColor = vtemu.VirtualColor
type VirtualCell = vtemu.VirtualCell

func NewVirtualTerminal(columns, rows int) *VirtualTerminal {
	return vtemu.New(columns, rows)
}

var _ Terminal = (*VirtualTerminal)(nil)
