package gicodingagent

import (
	"context"

	tmux "github.com/nowa/gi/gi-coding-agent/internal/tmux"
)

type TmuxOptionReader = tmux.OptionReader

func CheckTmuxKeyboardSetup(ctx context.Context, reader TmuxOptionReader) string {
	return tmux.CheckKeyboardSetup(ctx, reader)
}

func DefaultTmuxOptionReader(ctx context.Context, option string) (string, bool) {
	return tmux.DefaultOptionReader(ctx, option)
}
