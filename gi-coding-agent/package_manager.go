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

func ParsePackageSource(source string) PackageSource {
	trimmed := strings.TrimSpace(source)
	if strings.HasPrefix(trimmed, "npm:") {
		return PackageSource{Type: "npm", Source: trimmed, Path: strings.TrimSpace(strings.TrimPrefix(trimmed, "npm:"))}
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
	if source.Type == "npm" {
		return "npm:" + source.Path
	}
	return "local:" + filepath.Clean(source.Path)
}

func (m *DefaultPackageManager) Update(sources ...string) error {
	if len(sources) == 0 {
		sources = settingsPackagesToStrings(m.settingsManager.GetPackages())
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
		if options.Temporary && !source.Pinned {
			if _, err := os.Stat(packageDir); err == nil {
				if err := m.refreshGitPackage(packageDir); err != nil {
					return nil, err
				}
			} else if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		}
		resolved = append(resolved, packageDir)
	}
	return resolved, nil
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
