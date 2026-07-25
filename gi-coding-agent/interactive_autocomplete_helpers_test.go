package gicodingagent

import (
	"reflect"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func TestLoginProviderCompletionOptionsMergeAuthTypesAndSortByName(
	t *testing.T,
) {
	providers := []AuthSelectorProvider{
		{
			ID:       "same",
			Name:     "Same Provider",
			AuthType: string(llm.CredentialTypeAPIKey),
		},
		{
			ID:       "alpha",
			Name:     "Alpha Provider",
			AuthType: string(llm.CredentialTypeAPIKey),
		},
		{
			ID:       "same",
			Name:     "Same Provider",
			AuthType: string(llm.CredentialTypeOAuth),
		},
		{
			ID:       "same",
			Name:     "Same Provider",
			AuthType: string(llm.CredentialTypeOAuth),
		},
	}

	got := getLoginProviderCompletionOptions(providers)
	want := []loginProviderCompletionOption{
		{
			id:        "alpha",
			name:      "Alpha Provider",
			authTypes: []string{string(llm.CredentialTypeAPIKey)},
		},
		{
			id:   "same",
			name: "Same Provider",
			authTypes: []string{
				string(llm.CredentialTypeOAuth),
				string(llm.CredentialTypeAPIKey),
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("completion options = %#v, want %#v", got, want)
	}
	if search := getLoginProviderSearchText(got[1]); !strings.Contains(search, "oauth subscription") ||
		!strings.Contains(search, "api_key API key") {
		t.Fatalf("provider search text = %q", search)
	}
	if description := formatLoginProviderCompletionDescription(got[1]); description != "Same Provider · subscription/API key" {
		t.Fatalf("provider description = %q", description)
	}
}

func TestCreateFuzzyAutocompleteItemsReturnsTypedMatchesOrNil(
	t *testing.T,
) {
	options := []loginProviderCompletionOption{
		{
			id:        "alpha",
			name:      "Alpha",
			authTypes: []string{string(llm.CredentialTypeAPIKey)},
		},
		{
			id:        "bravo",
			name:      "Bravo",
			authTypes: []string{string(llm.CredentialTypeOAuth)},
		},
	}
	toItem := func(
		option loginProviderCompletionOption,
	) gitui.AutocompleteItem {
		return gitui.AutocompleteItem{
			Value: option.id,
			Label: option.name,
		}
	}
	got := createFuzzyAutocompleteItems(
		options,
		"brav sub",
		getLoginProviderSearchText,
		toItem,
	)
	if len(got) != 1 ||
		got[0].Value != "bravo" ||
		got[0].Label != "Bravo" {
		t.Fatalf("fuzzy completions = %#v", got)
	}
	if missing := createFuzzyAutocompleteItems(
		options,
		"missing",
		getLoginProviderSearchText,
		toItem,
	); missing != nil {
		t.Fatalf("missing completions = %#v, want nil", missing)
	}
}

func TestCLIInteractiveTUIHostLoginSlashArgumentAutocompletePiStyle(
	t *testing.T,
) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	t.Cleanup(func() {
		_ = runtimeHost.Dispose()
	})
	host, err := NewCLIInteractiveTUIHost(
		CLIInteractiveTUIHostOptions{RuntimeHost: runtimeHost},
	)
	if err != nil {
		t.Fatal(err)
	}
	loginCommand, ok := slashCommandByName(
		host.autocompleteSlashCommands(),
		"login",
	)
	if !ok || loginCommand.GetArgumentCompletions == nil {
		t.Fatalf(
			"/login command should expose argument completions: %#v ok=%v",
			loginCommand,
			ok,
		)
	}
	items := loginCommand.GetArgumentCompletions("openai")
	for _, item := range items {
		if item.Value == "openai" {
			if item.Label != "openai" ||
				!strings.Contains(item.Description, "OpenAI") ||
				!strings.Contains(item.Description, "API key") {
				t.Fatalf("/login completion = %#v", item)
			}
			return
		}
	}
	t.Fatalf("/login completions missing openai: %#v", items)
}
