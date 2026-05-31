package gicodingagent

import "github.com/nowa/gi/gi-coding-agent/internal/planmode"

type PlanTodoItem = planmode.PlanTodoItem

func IsSafePlanCommand(command string) bool {
	return planmode.IsSafePlanCommand(command)
}

func CleanPlanStepText(text string) string {
	return planmode.CleanPlanStepText(text)
}

func ExtractPlanTodoItems(message string) []PlanTodoItem {
	return planmode.ExtractPlanTodoItems(message)
}

func ExtractDoneSteps(message string) []int {
	return planmode.ExtractDoneSteps(message)
}

func MarkCompletedPlanSteps(message string, items []PlanTodoItem) int {
	return planmode.MarkCompletedPlanSteps(message, items)
}
