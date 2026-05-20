package gicodingagent

import (
	"reflect"
	"strings"
	"testing"
)

func TestPlanModeSafeCommands(t *testing.T) {
	for _, command := range []string{
		"ls -la", "cat file.txt", "head -n 10 file.txt", "tail -f log.txt", "grep pattern file", "find . -name '*.ts'",
		"git status", "git log --oneline", "git diff", "git branch",
		"npm list", "npm outdated", "yarn info react",
		"pwd", "echo hello", "wc -l file.txt", "du -sh .", "df -h",
		"  ls -la",
	} {
		if !IsSafePlanCommand(command) {
			t.Fatalf("IsSafePlanCommand(%q) = false, want true", command)
		}
	}
}

func TestPlanModeBlocksUnsafeCommands(t *testing.T) {
	for _, command := range []string{
		"rm file.txt", "rm -rf dir", "mv old new", "cp src dst", "mkdir newdir", "touch newfile",
		"git add .", "git commit -m 'msg'", "git push", "git checkout main", "git reset --hard",
		"npm install lodash", "yarn add react", "pip install requests", "brew install node",
		"echo hello > file.txt", "cat foo >> bar", ">file.txt",
		"sudo rm -rf /", "kill -9 1234", "reboot",
		"vim file.txt", "nano file.txt", "code .",
		"unknown-command", "my-script.sh", "  rm file",
	} {
		if IsSafePlanCommand(command) {
			t.Fatalf("IsSafePlanCommand(%q) = true, want false", command)
		}
	}
}

func TestCleanPlanStepText(t *testing.T) {
	cases := map[string]string{
		"**bold text**":                "Bold text",
		"*italic text*":                "Italic text",
		"run `npm install`":            "Npm install",
		"check the `config.json` file": "Config.json file",
		"Create the new file":          "New file",
		"Run the tests":                "Tests",
		"Check the status":             "Status",
		"update config":                "Config",
		"multiple   spaces   here":     "Multiple spaces here",
	}
	for input, want := range cases {
		if got := CleanPlanStepText(input); got != want {
			t.Fatalf("CleanPlanStepText(%q) = %q, want %q", input, got, want)
		}
	}
	longText := "This is a very long step description that exceeds the maximum allowed length for display"
	if got := CleanPlanStepText(longText); len(got) != 50 || !strings.HasSuffix(got, "...") {
		t.Fatalf("long clean text = %q len=%d", got, len(got))
	}
}

func TestExtractPlanTodoItems(t *testing.T) {
	message := "Here's what we'll do:\n\nPlan:\n1. First step here\n2. Second step here\n3. Third step here"
	items := ExtractPlanTodoItems(message)
	if len(items) != 3 || items[0].Step != 1 || items[0].Text != "First step here" || items[0].Completed {
		t.Fatalf("items = %#v", items)
	}
	if got := ExtractPlanTodoItems("**Plan:**\n1. Do something"); len(got) != 1 {
		t.Fatalf("bold plan items = %#v", got)
	}
	if got := ExtractPlanTodoItems("Plan:\n1) First item\n2) Second item"); len(got) != 2 {
		t.Fatalf("paren plan items = %#v", got)
	}
	if got := ExtractPlanTodoItems("Here are some steps:\n1. First step\n2. Second step"); len(got) != 0 {
		t.Fatalf("items without plan = %#v", got)
	}
	filtered := ExtractPlanTodoItems("Plan:\n1. OK\n2. This is a proper step")
	if len(filtered) != 1 || !strings.Contains(filtered[0].Text, "proper") {
		t.Fatalf("filtered short items = %#v", filtered)
	}
	codeFiltered := ExtractPlanTodoItems("Plan:\n1. `npm install`\n2. Run the build process")
	if len(codeFiltered) != 1 {
		t.Fatalf("filtered code items = %#v", codeFiltered)
	}
}

func TestPlanDoneMarkers(t *testing.T) {
	if got := ExtractDoneSteps("I've completed the first step [DONE:1]"); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("single done = %#v", got)
	}
	if got := ExtractDoneSteps("Did steps [DONE:1] and [DONE:2] and [DONE:3]"); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("multiple done = %#v", got)
	}
	if got := ExtractDoneSteps("[done:1] [DONE:2] [Done:3]"); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("case done = %#v", got)
	}
	if got := ExtractDoneSteps("No markers here"); len(got) != 0 {
		t.Fatalf("empty done = %#v", got)
	}
	if got := ExtractDoneSteps("[DONE:abc] [DONE:] [DONE:1]"); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("malformed done = %#v", got)
	}
}

func TestMarkCompletedPlanSteps(t *testing.T) {
	items := []PlanTodoItem{{Step: 1, Text: "First"}, {Step: 2, Text: "Second"}, {Step: 3, Text: "Third"}}
	if count := MarkCompletedPlanSteps("[DONE:1] [DONE:3]", items); count != 2 {
		t.Fatalf("count = %d", count)
	}
	if !items[0].Completed || items[1].Completed || !items[2].Completed {
		t.Fatalf("items = %#v", items)
	}
	one := []PlanTodoItem{{Step: 1, Text: "First"}}
	if MarkCompletedPlanSteps("[DONE:1]", one) != 1 || MarkCompletedPlanSteps("no markers", one) != 0 {
		t.Fatalf("single marker behavior failed: %#v", one)
	}
	if count := MarkCompletedPlanSteps("[DONE:99]", one); count != 1 || !one[0].Completed {
		t.Fatalf("missing step count/items = %d %#v", count, one)
	}
	already := []PlanTodoItem{{Step: 1, Text: "First", Completed: true}}
	MarkCompletedPlanSteps("[DONE:1]", already)
	if !already[0].Completed {
		t.Fatalf("already completed item changed: %#v", already)
	}
}
