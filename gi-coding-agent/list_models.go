package gicodingagent

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

type listModelsRow struct {
	provider string
	model    string
	context  string
	maxOut   string
	thinking string
	images   string
}

func runCLIListModels(args Args, options CLIOptions) int {
	registry, _, _, err := newCLIModelRegistry(options, false)
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	search := ""
	if value, ok := args.ListModels.(string); ok {
		search = value
	}
	if err := WriteListModels(nonNilWriter(options.Stdout), nonNilWriter(options.Stderr), registry, search); err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	return 0
}

func WriteListModels(stdout, stderr io.Writer, registry *ModelRegistry, searchPattern string) error {
	if registry == nil {
		return fmt.Errorf("model registry is required")
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if loadError := registry.GetError(); loadError != "" && stderr != nil {
		_, _ = fmt.Fprintf(stderr, "Warning: errors loading models.json:\n%s\n", loadError)
	}

	models := registry.GetAvailable()
	if len(models) == 0 {
		_, _ = fmt.Fprintln(stdout, formatNoModelsAvailableMessage())
		return nil
	}
	if searchPattern != "" {
		models = gitui.FuzzyFilter(models, searchPattern, func(model llm.Model) string {
			return model.Provider + " " + model.ID
		})
	}
	if len(models) == 0 {
		_, _ = fmt.Fprintf(stdout, "No models matching %q\n", searchPattern)
		return nil
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].ID < models[j].ID
	})

	rows := make([]listModelsRow, 0, len(models))
	for _, model := range models {
		rows = append(rows, listModelsRow{
			provider: model.Provider,
			model:    model.ID,
			context:  formatModelTokenCount(model.ContextWindow),
			maxOut:   formatModelTokenCount(model.MaxTokens),
			thinking: yesNo(model.Reasoning),
			images:   yesNo(modelSupportsImageInput(model)),
		})
	}
	writeListModelsTable(stdout, rows)
	return nil
}

func writeListModelsTable(writer io.Writer, rows []listModelsRow) {
	headers := listModelsRow{
		provider: "provider",
		model:    "model",
		context:  "context",
		maxOut:   "max-out",
		thinking: "thinking",
		images:   "images",
	}
	widths := headers
	for _, row := range rows {
		widths.provider = wider(widths.provider, row.provider)
		widths.model = wider(widths.model, row.model)
		widths.context = wider(widths.context, row.context)
		widths.maxOut = wider(widths.maxOut, row.maxOut)
		widths.thinking = wider(widths.thinking, row.thinking)
		widths.images = wider(widths.images, row.images)
	}

	writeListModelsRow(writer, headers, widths)
	for _, row := range rows {
		writeListModelsRow(writer, row, widths)
	}
}

func writeListModelsRow(writer io.Writer, row, widths listModelsRow) {
	_, _ = fmt.Fprintln(writer, strings.Join([]string{
		padRight(row.provider, len(widths.provider)),
		padRight(row.model, len(widths.model)),
		padRight(row.context, len(widths.context)),
		padRight(row.maxOut, len(widths.maxOut)),
		padRight(row.thinking, len(widths.thinking)),
		padRight(row.images, len(widths.images)),
	}, "  "))
}

func wider(current, candidate string) string {
	if len(candidate) > len(current) {
		return candidate
	}
	return current
}

func padRight(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func formatModelTokenCount(count int) string {
	switch {
	case count >= 1_000_000:
		millions := float64(count) / 1_000_000
		if count%1_000_000 == 0 {
			return strconv.Itoa(count/1_000_000) + "M"
		}
		return strconv.FormatFloat(millions, 'f', 1, 64) + "M"
	case count >= 1_000:
		if count%1_000 == 0 {
			return strconv.Itoa(count/1_000) + "K"
		}
		thousands := float64(count) / 1_000
		return strconv.FormatFloat(thousands, 'f', 1, 64) + "K"
	default:
		return strconv.Itoa(count)
	}
}

func modelSupportsImageInput(model llm.Model) bool {
	for _, input := range model.Input {
		if input == "image" {
			return true
		}
	}
	return false
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
