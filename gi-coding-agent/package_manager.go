package gicodingagent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type PackageManagerOptions struct {
	CWD             string
	AgentDir        string
	SettingsManager *SettingsManager
	Operations      PackageManagerOperations
}

type PackageManagerOperations struct {
	RunCommand        func(command string, args []string, options PackageCommandOptions) error
	RunCommandCapture func(command string, args []string, options PackageCommandOptions) (string, error)
}

type PackageCommandOptions struct {
	CWD string
}

type ResolveExtensionSourcesOptions struct {
	Temporary bool
}

type VersionReleaseChecker func(currentVersion string, options VersionCheckOptions) (LatestPiRelease, bool)

type SelfUpdateOptions struct {
	PackageName         string
	CurrentVersion      string
	Force               bool
	Environment         InstallEnvironment
	VersionCheck        VersionReleaseChecker
	VersionCheckOptions VersionCheckOptions
}

type SelfUpdateResult struct {
	Updated     bool
	PackageName string
}

type PackageUpdateSuggestionError struct {
	Input      string
	Suggestion string
}

func (e PackageUpdateSuggestionError) Error() string {
	return "No matching package found for " + e.Input + ". Did you mean " + e.Suggestion + "?"
}

type PackageSource struct {
	Type   string
	Source string
	Repo   string
	Host   string
	Path   string
	Ref    string
	Pinned bool
}

type DefaultPackageManager struct {
	cwd             string
	agentDir        string
	settingsManager *SettingsManager
	operations      PackageManagerOperations
}

func NewDefaultPackageManager(options PackageManagerOptions) *DefaultPackageManager {
	operations := normalizePackageManagerOperations(options.Operations)
	settingsManager := options.SettingsManager
	if settingsManager == nil {
		settingsManager = NewSettingsManager(options.CWD, options.AgentDir)
	}
	return &DefaultPackageManager{
		cwd:             options.CWD,
		agentDir:        options.AgentDir,
		settingsManager: settingsManager,
		operations:      operations,
	}
}

func (m *DefaultPackageManager) ParseSource(source string) PackageSource {
	return ParsePackageSource(source)
}

func (m *DefaultPackageManager) GetPackageIdentity(source string) string {
	return PackageSourceIdentity(ParsePackageSource(source))
}

func (m *DefaultPackageManager) Install(source string, project bool) error {
	_, err := m.addSourceToSettings(source, project)
	return err
}

func (m *DefaultPackageManager) addSourceToSettings(source string, project bool) (bool, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return false, fmt.Errorf("missing install source")
	}
	if unsupportedPackageSource(source) {
		return false, unsupportedPackageSourceError(source)
	}
	baseDir := m.settingsBaseDir(project)
	stored := m.packageSettingsValue(source, baseDir)
	packages := m.settingsPackages(project)
	if !packageSettingsContains(packages, stored, source, m.cwd, baseDir) {
		packages = append(packages, stored)
	} else {
		return false, nil
	}
	values := make([]any, len(packages))
	for i, value := range packages {
		values[i] = value
	}
	if project {
		m.settingsManager.SetProjectPackages(values)
	} else {
		m.settingsManager.SetPackages(values)
	}
	return true, nil
}

func (m *DefaultPackageManager) Remove(source string, project bool) error {
	_, err := m.removeSourceFromSettings(source, project)
	return err
}

func (m *DefaultPackageManager) removeSourceFromSettings(source string, project bool) (bool, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return false, fmt.Errorf("missing remove source")
	}
	baseDir := m.settingsBaseDir(project)
	packages := m.settingsPackages(project)
	filtered := make([]string, 0, len(packages))
	removed := false
	for _, existing := range packages {
		if packageSettingsMatch(existing, source, m.cwd, baseDir) {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !removed {
		return false, nil
	}
	values := make([]any, len(filtered))
	for i, value := range filtered {
		values[i] = value
	}
	if project {
		m.settingsManager.SetProjectPackages(values)
	} else {
		m.settingsManager.SetPackages(values)
	}
	return true, nil
}

func (m *DefaultPackageManager) settingsPackages(project bool) []string {
	settings := m.settingsManager.global
	if project {
		settings = m.settingsManager.project
	}
	return settingsPackagesToStrings(settingsSlice(settings, "packages"))
}

func (m *DefaultPackageManager) globalNPMCommand() []string {
	return settingsStringSlice(m.settingsManager.global, "npmCommand")
}

func (m *DefaultPackageManager) packageUpdateSuggestion(source string) string {
	source = strings.TrimSpace(source)
	if source == "" || strings.Contains(source, ":") {
		return ""
	}
	for _, existing := range settingsPackagesToStrings(m.settingsManager.GetPackages()) {
		parsed := ParsePackageSource(existing)
		if parsed.Type == "git" && parsed.Host+"/"+parsed.Path == source {
			return existing
		}
	}
	return ""
}

func ParsePackageSource(source string) PackageSource {
	trimmed := strings.TrimSpace(source)
	if strings.HasPrefix(trimmed, "npm:") {
		return PackageSource{Type: "unsupported", Source: trimmed, Path: strings.TrimSpace(strings.TrimPrefix(trimmed, "npm:"))}
	}
	if gitSource, ok := ParseGitURL(trimmed); ok {
		return PackageSource{
			Type:   "git",
			Source: trimmed,
			Repo:   gitSource.Repo,
			Host:   gitSource.Host,
			Path:   gitSource.Path,
			Ref:    gitSource.Ref,
			Pinned: gitSource.Pinned,
		}
	}
	return PackageSource{Type: "local", Source: trimmed, Path: trimmed}
}

func PackageSourceIdentity(source PackageSource) string {
	if source.Type == "git" {
		return "git:" + source.Host + "/" + source.Path
	}
	if source.Type == "unsupported" {
		return "unsupported:" + source.Source
	}
	return "local:" + filepath.Clean(source.Path)
}

func (m *DefaultPackageManager) Update(sources ...string) error {
	if len(sources) == 0 {
		sources = settingsPackagesToStrings(m.settingsManager.GetPackages())
	} else {
		for _, sourceText := range sources {
			if suggestion := m.packageUpdateSuggestion(sourceText); suggestion != "" {
				return PackageUpdateSuggestionError{Input: sourceText, Suggestion: suggestion}
			}
		}
	}
	for _, sourceText := range sources {
		if unsupportedPackageSource(sourceText) {
			return unsupportedPackageSourceError(sourceText)
		}
	}
	for _, sourceText := range sources {
		source, ok := ParseGitURL(sourceText)
		if !ok || source.Pinned {
			continue
		}
		installedDir := gitPackageInstallPath(m.agentDir, source)
		if _, err := os.Stat(installedDir); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := m.refreshGitPackage(installedDir); err != nil {
			return err
		}
	}
	return nil
}

func unsupportedPackageSource(source string) bool {
	return ParsePackageSource(source).Type == "unsupported"
}

func unsupportedPackageSourceError(source string) error {
	return fmt.Errorf("unsupported package source %q: Gi packages support local paths and git URLs with gi.package.json; npm packages are not supported", strings.TrimSpace(source))
}

func (m *DefaultPackageManager) RunSelfUpdate(options SelfUpdateOptions) (SelfUpdateResult, error) {
	packageName := firstNonEmptyString(options.PackageName, DefaultCodingAgentPackageName)
	currentVersion := firstNonEmptyString(options.CurrentVersion, DefaultCodingAgentVersion)
	updatePackageName := packageName
	if !options.Force {
		checker := options.VersionCheck
		if checker == nil {
			checker = GetLatestPiRelease
		}
		release, ok := checker(currentVersion, options.VersionCheckOptions)
		if !ok || !IsNewerPackageVersion(release.Version, currentVersion) {
			return SelfUpdateResult{}, nil
		}
		if release.PackageName != "" {
			updatePackageName = release.PackageName
		}
	}

	npmCommand := m.globalNPMCommand()
	command := GetSelfUpdateCommand(packageName, options.Environment, npmCommand, updatePackageName)
	if command == nil {
		return SelfUpdateResult{}, fmt.Errorf("%s", GetSelfUpdateUnavailableInstruction(packageName, options.Environment, npmCommand, updatePackageName))
	}
	steps := command.Steps
	if len(steps) == 0 {
		steps = []SelfUpdateCommandStep{{Command: command.Command, Args: command.Args, Display: command.Display}}
	}
	for _, step := range steps {
		if err := m.operations.RunCommand(step.Command, step.Args, PackageCommandOptions{}); err != nil {
			return SelfUpdateResult{}, err
		}
	}
	return SelfUpdateResult{Updated: true, PackageName: updatePackageName}, nil
}

func (m *DefaultPackageManager) ResolveExtensionSources(sources []string, options ResolveExtensionSourcesOptions) ([]string, error) {
	resolved := make([]string, 0, len(sources))
	for _, sourceText := range sources {
		source, ok := ParseGitURL(sourceText)
		if !ok {
			continue
		}
		packageDir := gitPackageInstallPath(m.agentDir, source)
		if options.Temporary {
			packageDir = temporaryGitPackagePath(source)
		}
		offline := packageManagerOffline()
		if options.Temporary && !source.Pinned && !offline {
			if _, err := os.Stat(packageDir); err == nil {
				if err := m.refreshGitPackage(packageDir); err != nil {
					return nil, err
				}
			} else if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		} else if offline {
			if _, err := os.Stat(packageDir); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
		}
		resolved = append(resolved, packageDir)
	}
	return resolved, nil
}

func packageManagerOffline() bool {
	value := strings.TrimSpace(os.Getenv("GI_OFFLINE"))
	if value == "" {
		value = strings.TrimSpace(os.Getenv("PI_OFFLINE"))
	}
	return value == "1" || strings.EqualFold(value, "true")
}

func (m *DefaultPackageManager) refreshGitPackage(packageDir string) error {
	branch := "main"
	fetchArgs := []string{"fetch", "--prune", "--no-tags", "origin", "+refs/heads/" + branch + ":refs/remotes/origin/" + branch}
	if err := m.operations.RunCommand("git", fetchArgs, PackageCommandOptions{CWD: packageDir}); err != nil {
		return err
	}

	localHead, _ := m.operations.RunCommandCapture("git", []string{"rev-parse", "HEAD"}, PackageCommandOptions{CWD: packageDir})
	remoteHead, _ := m.operations.RunCommandCapture("git", []string{"rev-parse", "origin/" + branch}, PackageCommandOptions{CWD: packageDir})
	if strings.TrimSpace(localHead) != "" && strings.TrimSpace(localHead) == strings.TrimSpace(remoteHead) {
		return nil
	}

	if err := m.operations.RunCommand("git", []string{"reset", "--hard", "origin/" + branch}, PackageCommandOptions{CWD: packageDir}); err != nil {
		return err
	}
	return m.operations.RunCommand("git", []string{"clean", "-fdx"}, PackageCommandOptions{CWD: packageDir})
}

func normalizePackageManagerOperations(operations PackageManagerOperations) PackageManagerOperations {
	if operations.RunCommand == nil {
		operations.RunCommand = runPackageCommand
	}
	if operations.RunCommandCapture == nil {
		operations.RunCommandCapture = runPackageCommandCapture
	}
	return operations
}

func runPackageCommand(command string, args []string, options PackageCommandOptions) error {
	cmd := exec.Command(command, args...)
	cmd.Dir = options.CWD
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Command failed: %s %s\n%s", command, strings.Join(args, " "), string(output))
	}
	return nil
}

func runPackageCommandCapture(command string, args []string, options PackageCommandOptions) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = options.CWD
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Command failed: %s %s\n%s", command, strings.Join(args, " "), string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func settingsPackagesToStrings(values []any) []string {
	sources := make([]string, 0, len(values))
	for _, value := range values {
		if source, ok := value.(string); ok && strings.TrimSpace(source) != "" {
			sources = append(sources, source)
		}
	}
	return sources
}

func (m *DefaultPackageManager) settingsBaseDir(project bool) string {
	if project {
		return filepath.Join(m.cwd, ConfigDirName)
	}
	return m.agentDir
}

func (m *DefaultPackageManager) packageSettingsValue(source, baseDir string) string {
	parsed := ParsePackageSource(source)
	if parsed.Type != "local" {
		return source
	}
	absolute := ResolveToCwd(parsed.Path, m.cwd)
	if relative, err := filepath.Rel(baseDir, absolute); err == nil {
		cleaned := filepath.Clean(relative)
		if cleaned != "." && !strings.HasPrefix(cleaned, ".") {
			return "." + string(filepath.Separator) + cleaned
		}
		return cleaned
	}
	return absolute
}

func packageSettingsContains(existing []string, stored, source, cwd, baseDir string) bool {
	for _, value := range existing {
		if packageSettingsMatch(value, source, cwd, baseDir) || packageSettingsMatch(value, stored, cwd, baseDir) {
			return true
		}
	}
	return false
}

func packageSettingsMatch(existing, source, cwd, baseDir string) bool {
	existingParsed := ParsePackageSource(existing)
	sourceParsed := ParsePackageSource(source)
	if existingParsed.Type != sourceParsed.Type {
		return false
	}
	if existingParsed.Type == "local" {
		return packageLocalIdentity(existingParsed.Path, baseDir) == packageLocalIdentity(sourceParsed.Path, cwd)
	}
	return PackageSourceIdentity(existingParsed) == PackageSourceIdentity(sourceParsed)
}

func packageLocalIdentity(path, baseDir string) string {
	absolute := ResolveToCwd(strings.TrimRight(path, `/\`), baseDir)
	if realPath, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = realPath
	}
	return filepath.Clean(absolute)
}

func gitPackageInstallPath(agentDir string, source GitSource) string {
	elements := []string{agentDir, "git", source.Host}
	elements = append(elements, strings.Split(source.Path, "/")...)
	return filepath.Join(elements...)
}

func temporaryGitPackagePath(source GitSource) string {
	cacheKey := "git-" + source.Host + "-" + source.Path
	sum := sha256.Sum256([]byte(cacheKey))
	hash := hex.EncodeToString(sum[:])[:8]
	elements := []string{os.TempDir(), "pi-extensions", "git-" + source.Host, hash}
	elements = append(elements, strings.Split(source.Path, "/")...)
	return filepath.Join(elements...)
}
