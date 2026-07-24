package gicodingagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

type ProtocolExtensionProcessSupervisor struct {
	Host            *RPCSessionHost
	StartTimeout    time.Duration
	ShutdownTimeout time.Duration
	processes       []*ProtocolExtensionProcess
}

type ProtocolExtensionProcess struct {
	Spec ProtocolPackageProcessExtension

	cmd                   *exec.Cmd
	cancel                context.CancelFunc
	host                  *RPCSessionHost
	stdin                 io.WriteCloser
	stderrTail            *processOutputTail
	processorMu           sync.RWMutex
	processor             *RPCLineProcessor
	writeMu               sync.Mutex
	eventMu               sync.Mutex
	eventSeq              int
	readyMu               sync.Mutex
	readyMarked           bool
	readyOnce             sync.Once
	ready                 chan error
	done                  chan error
	viewTreeUnsubscribe   func()
	ownedViewTreeMountsMu sync.Mutex
	ownedViewTreeMounts   map[string]bool
	ownedTUIStatusMu      sync.Mutex
	ownedTUIStatusKeys    map[string]bool
	ownedTUIStateMu       sync.Mutex
	ownedTUITitle         bool
	ownedTUIWorking       bool
	ownedTUIThinkingLabel bool
	cleanupRuntimeOnce    sync.Once
}

func NewProtocolExtensionProcessSupervisor(host *RPCSessionHost, specs []ProtocolPackageProcessExtension) *ProtocolExtensionProcessSupervisor {
	supervisor := &ProtocolExtensionProcessSupervisor{
		Host:            host,
		StartTimeout:    2 * time.Second,
		ShutdownTimeout: time.Second,
	}
	for _, spec := range specs {
		if len(spec.Command) == 0 || strings.TrimSpace(spec.ID) == "" {
			continue
		}
		supervisor.processes = append(supervisor.processes, &ProtocolExtensionProcess{Spec: spec})
	}
	return supervisor
}

func (s *ProtocolExtensionProcessSupervisor) Start(ctx context.Context) error {
	if s == nil {
		return nil
	}
	for _, process := range s.processes {
		if err := process.Start(ctx, s.Host, s.startTimeout()); err != nil {
			_ = s.Stop(context.Background())
			return err
		}
	}
	return nil
}

func (s *ProtocolExtensionProcessSupervisor) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	var errs []string
	for _, process := range s.processes {
		if err := process.Stop(ctx, s.shutdownTimeout()); err != nil {
			errs = append(errs, process.Spec.ID+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (s *ProtocolExtensionProcessSupervisor) BindSession(session *AgentSession) {
	if s == nil || s.Host == nil || session == nil {
		return
	}
	s.Host.replaceSession(session)
}

func (s *ProtocolExtensionProcessSupervisor) EmitEvent(method string, params map[string]any) error {
	if s == nil {
		return nil
	}
	var errs []string
	for _, process := range s.processes {
		if err := process.EmitEvent(method, params); err != nil {
			errs = append(errs, process.Spec.ID+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (s *ProtocolExtensionProcessSupervisor) EmitSessionEvent(event ProtocolSessionEvent) error {
	if s == nil || strings.TrimSpace(event.Type) == "" {
		return nil
	}
	return s.EmitEvent(event.Type, protocolSessionEventProcessParams(event))
}

func (s *ProtocolExtensionProcessSupervisor) EmitTerminalInput(data string) error {
	return s.emitEventToCapability(CapabilityTUITerminalInput, "tui.terminal_input", map[string]any{"data": data})
}

func (s *ProtocolExtensionProcessSupervisor) EmitUserBash(ctx context.Context, params map[string]any) (*BashResult, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	var errs []string
	for _, process := range s.processes {
		if !capabilityAllowed(CapabilityBashIntercept, process.Spec.Capabilities, true) {
			continue
		}
		raw, err := process.RequestEvent(ctx, ProtocolEventUserBash, cloneMapAny(params))
		if err != nil {
			errs = append(errs, process.Spec.ID+": "+err.Error())
			continue
		}
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var result ProtocolEventResult
		if err := json.Unmarshal(raw, &result); err != nil {
			errs = append(errs, process.Spec.ID+": invalid user_bash response: "+err.Error())
			continue
		}
		if result.BashResultSet || result.BashResult != nil {
			if result.BashResult == nil {
				return &BashResult{}, true, nil
			}
			bashResult := *result.BashResult
			return &bashResult, true, nil
		}
	}
	if len(errs) > 0 {
		return nil, false, errors.New(strings.Join(errs, "; "))
	}
	return nil, false, nil
}

func (s *ProtocolExtensionProcessSupervisor) DiscoverResources(ctx context.Context, reason, cwd string) (ResourceExtension, error) {
	if s == nil {
		return ResourceExtension{}, nil
	}
	var combined ResourceExtension
	var errs []string
	for _, process := range s.processes {
		if !capabilityAllowed(CapabilityResourcesDiscover, process.Spec.Capabilities, true) {
			continue
		}
		raw, err := process.RequestEvent(ctx, ProtocolEventResourcesDiscover, map[string]any{
			"reason": reason,
			"cwd":    cwd,
		})
		if err != nil {
			errs = append(errs, process.Spec.ID+": "+err.Error())
			continue
		}
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var result ProtocolEventResult
		if err := json.Unmarshal(raw, &result); err != nil {
			errs = append(errs, process.Spec.ID+": invalid resources_discover response: "+err.Error())
			continue
		}
		if !eventResultHasResources(result) {
			continue
		}
		resources := processDiscoveredResourcesWithMetadata(process, result.Resources)
		combined.SkillPaths = append(combined.SkillPaths, resources.SkillPaths...)
		combined.PromptPaths = append(combined.PromptPaths, resources.PromptPaths...)
		combined.ThemePaths = append(combined.ThemePaths, resources.ThemePaths...)
	}
	if len(errs) > 0 {
		return combined, errors.New(strings.Join(errs, "; "))
	}
	return combined, nil
}

func eventResultHasResources(result ProtocolEventResult) bool {
	return result.ResourcesSet ||
		len(result.Resources.SkillPaths) > 0 ||
		len(result.Resources.PromptPaths) > 0 ||
		len(result.Resources.ThemePaths) > 0
}

func processDiscoveredResourcesWithMetadata(process *ProtocolExtensionProcess, resources ResourceExtension) ResourceExtension {
	if process == nil {
		return resources
	}
	source := process.Spec.Metadata
	if source.Path == "" {
		source.Path = process.Spec.Path
	}
	if source.Source == "" {
		source.Source = "process:" + process.Spec.ID
	}
	resources = resourceExtensionWithSourceDefaults(resources, source)
	baseDir := process.Spec.PackageDir
	for index := range resources.SkillPaths {
		resources.SkillPaths[index] = resolveProcessResourceSkillPath(resources.SkillPaths[index], baseDir)
	}
	for index := range resources.PromptPaths {
		resources.PromptPaths[index] = resolveProcessResourcePromptPath(resources.PromptPaths[index], baseDir)
	}
	for index := range resources.ThemePaths {
		resources.ThemePaths[index] = resolveProcessResourceThemePath(resources.ThemePaths[index], baseDir)
	}
	return resources
}

func resolveProcessResourceSkillPath(path ResourceSkillPath, baseDir string) ResourceSkillPath {
	resolved := ResolveToCwd(path.Path, baseDir)
	path.Path = resolved
	path.Metadata.Path = resolved
	return path
}

func resolveProcessResourcePromptPath(path ResourcePromptPath, baseDir string) ResourcePromptPath {
	resolved := ResolveToCwd(path.Path, baseDir)
	path.Path = resolved
	path.Metadata.Path = resolved
	return path
}

func resolveProcessResourceThemePath(path ResourceThemePath, baseDir string) ResourceThemePath {
	resolved := ResolveToCwd(path.Path, baseDir)
	path.Path = resolved
	path.Metadata.Path = resolved
	return path
}

func (s *ProtocolExtensionProcessSupervisor) emitEventToCapability(capability, method string, params map[string]any) error {
	if s == nil {
		return nil
	}
	var errs []string
	for _, process := range s.processes {
		if !capabilityAllowed(capability, process.Spec.Capabilities, true) {
			continue
		}
		if err := process.EmitEvent(method, params); err != nil {
			errs = append(errs, process.Spec.ID+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (s *ProtocolExtensionProcessSupervisor) Processes() []*ProtocolExtensionProcess {
	if s == nil {
		return nil
	}
	return append([]*ProtocolExtensionProcess(nil), s.processes...)
}

func (s *ProtocolExtensionProcessSupervisor) startTimeout() time.Duration {
	if s == nil || s.StartTimeout <= 0 {
		return 2 * time.Second
	}
	return s.StartTimeout
}

func (s *ProtocolExtensionProcessSupervisor) shutdownTimeout() time.Duration {
	if s == nil || s.ShutdownTimeout <= 0 {
		return time.Second
	}
	return s.ShutdownTimeout
}

func protocolSessionEventProcessParams(event ProtocolSessionEvent) map[string]any {
	params := map[string]any{}
	if event.Reason != "" {
		params["reason"] = event.Reason
	}
	if event.TargetSessionFile != "" {
		params["targetSessionFile"] = event.TargetSessionFile
	}
	if event.PreviousSessionFile != "" {
		params["previousSessionFile"] = event.PreviousSessionFile
	}
	if event.EntryID != "" {
		params["entryId"] = event.EntryID
	}
	if event.Position != "" {
		params["position"] = event.Position
	}
	return params
}

func (p *ProtocolExtensionProcess) Start(ctx context.Context, host *RPCSessionHost, timeout time.Duration) error {
	if p == nil {
		return nil
	}
	if len(p.Spec.Command) == 0 {
		return errors.New("extension process command is required")
	}
	if strings.TrimSpace(p.Spec.Transport) != "" && p.Spec.Transport != "stdio-ndjson" {
		return errors.New("unsupported extension process transport " + p.Spec.Transport)
	}
	if strings.TrimSpace(p.Spec.Protocol) != "" && p.Spec.Protocol != "gi-ext-rpc@1" {
		return errors.New("unsupported extension process protocol " + p.Spec.Protocol)
	}
	p.host = host
	processCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(processCtx, p.Spec.Command[0], p.Spec.Command[1:]...)
	if p.Spec.PackageDir != "" {
		cmd.Dir = p.Spec.PackageDir
	}
	configureHostProcessCommand(cmd)
	cmd.Cancel = func() error {
		return killHostProcess(cmd.Process)
	}
	cmd.Env = protocolExtensionProcessEnv(p.Spec)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}
	stderrTail := &processOutputTail{limit: 4096}
	p.cmd = cmd
	p.cancel = cancel
	p.stdin = stdin
	p.stderrTail = stderrTail
	p.ready = make(chan error, 1)
	p.done = make(chan error, 1)
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}
	p.subscribeViewTreeEvents(host)
	go p.readStderr(stderr, host)
	go p.readLoop(processCtx, stdout, host)
	go func() {
		err := cmd.Wait()
		p.unsubscribeViewTreeEvents()
		p.cleanupOwnedViewTreeMounts()
		p.cleanupOwnedTUIStatuses()
		p.cleanupOwnedTUIState()
		p.cleanupRuntimeRegistrations()
		if p.readyWasMarked() {
			p.emitExitDiagnostic(err)
		}
		p.done <- err
		close(p.done)
		p.markReady(p.exitBeforeHelloError(err))
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-p.ready:
		return err
	case err := <-p.done:
		return p.exitBeforeHelloError(err)
	case <-timer.C:
		return p.helloTimeoutError(p.kill())
	case <-ctx.Done():
		_ = p.kill()
		return ctx.Err()
	}
}

func (p *ProtocolExtensionProcess) Stop(ctx context.Context, timeout time.Duration) error {
	if p == nil || p.cmd == nil {
		return nil
	}
	_ = p.EmitEvent(ProtocolEventSessionShutdown, map[string]any{
		"reason": "shutdown",
	})
	p.unsubscribeViewTreeEvents()
	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-p.done:
		p.cancelProcess()
		return normalizeProcessExitError(err)
	case <-timer.C:
		return p.shutdownTimeoutError(p.kill())
	case <-ctx.Done():
		_ = p.kill()
		return ctx.Err()
	}
}

func (p *ProtocolExtensionProcess) EmitEvent(method string, params map[string]any) error {
	if p == nil || p.cmd == nil {
		return nil
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return errors.New("extension process event method is required")
	}
	return p.writeJSON(map[string]any{
		"type":     "event",
		"protocol": "gi-ext-rpc@1",
		"eventSeq": p.nextEventSeq(),
		"method":   method,
		"params":   cloneMapAny(params),
	})
}

func (p *ProtocolExtensionProcess) RequestEvent(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if p == nil || p.cmd == nil {
		return nil, nil
	}
	processor := p.processorSnapshot()
	if processor == nil {
		return nil, errors.New("extension process is not ready")
	}
	return processor.callExtension(ctx, method, params)
}

func (p *ProtocolExtensionProcess) nextEventSeq() int {
	p.eventMu.Lock()
	defer p.eventMu.Unlock()
	p.eventSeq++
	return p.eventSeq
}

func (p *ProtocolExtensionProcess) readLoop(ctx context.Context, stdout io.Reader, host *RPCSessionHost) {
	processor := &RPCLineProcessor{
		Host:                host,
		Runtime:             protocolExtensionRuntimeFromHost(host),
		SourceInfo:          p.sourceInfo(),
		AllowedCapabilities: append([]string(nil), p.Spec.Capabilities...),
		EnforceCapabilities: true,
		OnBeforeHostAction: func(request HostActionRequest) {
			p.prepareHostAction(request)
		},
		OnHostAction: func(request HostActionRequest, response HostActionResponse) {
			p.recordHostAction(request, response)
		},
		WriteLine: func(line string) {
			_ = p.writeLine(line)
		},
	}
	p.setProcessor(processor)
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimSuffix(line, []byte{'\n'})
			line = bytes.TrimSuffix(line, []byte{'\r'})
			text := strings.TrimSpace(string(line))
			if text != "" {
				if protocolExtensionLineType(text) == "hello" {
					processor.HandleLine(ctx, text)
					p.markReady(nil)
				} else {
					processor.HandleLine(ctx, text)
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				p.markReady(err)
			}
			return
		}
	}
}

func (p *ProtocolExtensionProcess) readStderr(stderr io.Reader, host *RPCSessionHost) {
	if p == nil || stderr == nil {
		return
	}
	runtime := protocolExtensionRuntimeFromHost(host)
	reader := bufio.NewReader(stderr)
	for {
		chunk, err := reader.ReadString('\n')
		if chunk != "" {
			if p.stderrTail != nil {
				_, _ = p.stderrTail.Write([]byte(chunk))
			}
			message := strings.TrimSpace(chunk)
			if message != "" && runtime != nil {
				runtime.emitExtensionError(ProtocolExtensionError{
					ExtensionPath: p.sourceInfo().Path,
					Event:         "stderr",
					Error:         message,
				})
			}
		}
		if err != nil {
			return
		}
	}
}

func (p *ProtocolExtensionProcess) subscribeViewTreeEvents(host *RPCSessionHost) {
	if p == nil || host == nil || host.ViewTreeHost == nil {
		return
	}
	if !hasViewTreeEventCapability(p.Spec.Capabilities) {
		return
	}
	p.viewTreeUnsubscribe = host.ViewTreeHost.OnEvent(func(event ViewTreeEvent) {
		if !p.ownsViewTreeMount(event.MountID) {
			return
		}
		_ = p.EmitEvent("tui.event", map[string]any{
			"mountId": event.MountID,
			"nodeId":  event.NodeID,
			"event":   event.Event,
			"data":    cloneMapAny(event.Data),
		})
	})
}

func hasViewTreeEventCapability(capabilities []string) bool {
	for _, capability := range []string{CapabilityTUIWidget, CapabilityTUIHeader, CapabilityTUIFooter, CapabilityTUIOverlay, CapabilityTUIEditor} {
		if capabilityAllowed(capability, capabilities, true) {
			return true
		}
	}
	return false
}

func (p *ProtocolExtensionProcess) unsubscribeViewTreeEvents() {
	if p == nil || p.viewTreeUnsubscribe == nil {
		return
	}
	p.viewTreeUnsubscribe()
	p.viewTreeUnsubscribe = nil
}

func (p *ProtocolExtensionProcess) prepareHostAction(request HostActionRequest) {
	if p == nil {
		return
	}
	switch strings.TrimSpace(request.Method) {
	case "host.tui.mount":
		var params hostTUIMountParams
		if err := json.Unmarshal(request.Params, &params); err == nil && strings.TrimSpace(params.MountID) != "" {
			p.setViewTreeMountOwned(params.MountID, true)
		}
	case "host.tui.title":
		var params hostTUITitleParams
		if err := json.Unmarshal(request.Params, &params); err == nil && strings.TrimSpace(params.Title) != "" {
			p.setTUITitleOwned(true)
		}
	case "host.tui.working":
		var params hostTUIWorkingParams
		if err := json.Unmarshal(request.Params, &params); err == nil && hostTUIWorkingParamsHasUpdate(params) {
			p.setTUIWorkingOwned(true)
		}
	case "host.tui.thinking_label":
		var params hostTUIThinkingLabelParams
		if err := json.Unmarshal(request.Params, &params); err == nil && strings.TrimSpace(params.Label) != "" && !params.Reset {
			p.setTUIThinkingLabelOwned(true)
		}
	}
}

func (p *ProtocolExtensionProcess) recordHostAction(request HostActionRequest, response HostActionResponse) {
	if p == nil {
		return
	}
	if response.Error != nil {
		switch strings.TrimSpace(request.Method) {
		case "host.tui.mount":
			var params hostTUIMountParams
			if err := json.Unmarshal(request.Params, &params); err == nil && strings.TrimSpace(params.MountID) != "" {
				p.setViewTreeMountOwned(params.MountID, false)
			}
		case "host.tui.title":
			var params hostTUITitleParams
			if err := json.Unmarshal(request.Params, &params); err == nil && strings.TrimSpace(params.Title) != "" {
				p.setTUITitleOwned(false)
			}
		case "host.tui.working":
			var params hostTUIWorkingParams
			if err := json.Unmarshal(request.Params, &params); err == nil && hostTUIWorkingParamsHasUpdate(params) {
				p.setTUIWorkingOwned(false)
			}
		case "host.tui.thinking_label":
			var params hostTUIThinkingLabelParams
			if err := json.Unmarshal(request.Params, &params); err == nil && strings.TrimSpace(params.Label) != "" && !params.Reset {
				p.setTUIThinkingLabelOwned(false)
			}
		}
		return
	}
	switch strings.TrimSpace(request.Method) {
	case "host.tui.mount":
		var params hostTUIMountParams
		if err := json.Unmarshal(request.Params, &params); err == nil && strings.TrimSpace(params.MountID) != "" {
			p.setViewTreeMountOwned(params.MountID, true)
		}
	case "host.tui.unmount":
		var params hostTUIUnmountParams
		if err := json.Unmarshal(request.Params, &params); err == nil && strings.TrimSpace(params.MountID) != "" {
			p.setViewTreeMountOwned(params.MountID, false)
		}
	case "host.tui.status":
		var params hostTUIStatusParams
		if err := json.Unmarshal(request.Params, &params); err == nil && strings.TrimSpace(params.Key) != "" {
			p.setTUIStatusOwned(params.Key, strings.TrimSpace(params.Text) != "")
		}
	case "host.tui.title":
		var params hostTUITitleParams
		if err := json.Unmarshal(request.Params, &params); err == nil {
			p.setTUITitleOwned(strings.TrimSpace(params.Title) != "")
		}
	case "host.tui.working":
		var params hostTUIWorkingParams
		if err := json.Unmarshal(request.Params, &params); err == nil && hostTUIWorkingParamsHasUpdate(params) {
			p.setTUIWorkingOwned(true)
		}
	case "host.tui.thinking_label":
		var params hostTUIThinkingLabelParams
		if err := json.Unmarshal(request.Params, &params); err == nil {
			p.setTUIThinkingLabelOwned(strings.TrimSpace(params.Label) != "" && !params.Reset)
		}
	}
}

func hostTUIWorkingParamsHasUpdate(params hostTUIWorkingParams) bool {
	return params.Message != nil ||
		params.ResetMessage ||
		params.Visible != nil ||
		params.Indicator != nil ||
		params.ResetIndicator
}

func (p *ProtocolExtensionProcess) setViewTreeMountOwned(mountID string, owned bool) {
	if p == nil {
		return
	}
	mountID = strings.TrimSpace(mountID)
	if mountID == "" {
		return
	}
	p.ownedViewTreeMountsMu.Lock()
	defer p.ownedViewTreeMountsMu.Unlock()
	if p.ownedViewTreeMounts == nil {
		p.ownedViewTreeMounts = map[string]bool{}
	}
	if owned {
		p.ownedViewTreeMounts[mountID] = true
		return
	}
	delete(p.ownedViewTreeMounts, mountID)
}

func (p *ProtocolExtensionProcess) ownsViewTreeMount(mountID string) bool {
	if p == nil {
		return false
	}
	p.ownedViewTreeMountsMu.Lock()
	defer p.ownedViewTreeMountsMu.Unlock()
	return p.ownedViewTreeMounts[strings.TrimSpace(mountID)]
}

func (p *ProtocolExtensionProcess) cleanupOwnedViewTreeMounts() {
	if p == nil || p.host == nil || p.host.ViewTreeHost == nil {
		return
	}
	mountIDs := p.consumeOwnedViewTreeMountIDs()
	for _, mountID := range mountIDs {
		p.host.ViewTreeHost.Unmount(mountID)
	}
}

func (p *ProtocolExtensionProcess) setTUIStatusOwned(key string, owned bool) {
	if p == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	p.ownedTUIStatusMu.Lock()
	defer p.ownedTUIStatusMu.Unlock()
	if p.ownedTUIStatusKeys == nil {
		p.ownedTUIStatusKeys = map[string]bool{}
	}
	if owned {
		p.ownedTUIStatusKeys[key] = true
		return
	}
	delete(p.ownedTUIStatusKeys, key)
}

func (p *ProtocolExtensionProcess) cleanupOwnedTUIStatuses() {
	if p == nil || p.host == nil {
		return
	}
	keys := p.consumeOwnedTUIStatusKeys()
	for _, key := range keys {
		if p.host.ViewTreeHost != nil {
			_ = p.host.ViewTreeHost.SetStatus(key, "", 0)
		}
		if p.host.TUIStatus != nil {
			_ = p.host.TUIStatus.SetTUIStatus(key, "")
		}
	}
}

func (p *ProtocolExtensionProcess) setTUITitleOwned(owned bool) {
	if p == nil {
		return
	}
	p.ownedTUIStateMu.Lock()
	defer p.ownedTUIStateMu.Unlock()
	p.ownedTUITitle = owned
}

func (p *ProtocolExtensionProcess) setTUIWorkingOwned(owned bool) {
	if p == nil {
		return
	}
	p.ownedTUIStateMu.Lock()
	defer p.ownedTUIStateMu.Unlock()
	p.ownedTUIWorking = owned
}

func (p *ProtocolExtensionProcess) setTUIThinkingLabelOwned(owned bool) {
	if p == nil {
		return
	}
	p.ownedTUIStateMu.Lock()
	defer p.ownedTUIStateMu.Unlock()
	p.ownedTUIThinkingLabel = owned
}

func (p *ProtocolExtensionProcess) cleanupOwnedTUIState() {
	if p == nil || p.host == nil {
		return
	}
	owned := p.consumeOwnedTUIState()
	if owned.title && p.host.TUITitle != nil {
		_ = p.host.TUITitle.SetTUITitle("")
	}
	if owned.working && p.host.TUIWorking != nil {
		_ = p.host.TUIWorking.SetTUIWorking(TUIWorkingUpdate{
			ResetMessage:   true,
			Visible:        true,
			VisibleSet:     true,
			ResetIndicator: true,
		})
	}
	if owned.thinkingLabel && p.host.TUIThinkingLabel != nil {
		_ = p.host.TUIThinkingLabel.SetHiddenThinkingLabel("")
	}
}

func (p *ProtocolExtensionProcess) cleanupRuntimeRegistrations() {
	if p == nil {
		return
	}
	p.cleanupRuntimeOnce.Do(func() {
		processor := p.processorSnapshot()
		if processor == nil || processor.Runtime == nil {
			return
		}
		processor.Runtime.RemoveSource(p.sourceInfo())
	})
}

func (p *ProtocolExtensionProcess) setProcessor(processor *RPCLineProcessor) {
	if p == nil {
		return
	}
	p.processorMu.Lock()
	p.processor = processor
	p.processorMu.Unlock()
}

func (p *ProtocolExtensionProcess) processorSnapshot() *RPCLineProcessor {
	if p == nil {
		return nil
	}
	p.processorMu.RLock()
	processor := p.processor
	p.processorMu.RUnlock()
	return processor
}

func (p *ProtocolExtensionProcess) consumeOwnedViewTreeMountIDs() []string {
	if p == nil {
		return nil
	}
	p.ownedViewTreeMountsMu.Lock()
	defer p.ownedViewTreeMountsMu.Unlock()
	mountIDs := make([]string, 0, len(p.ownedViewTreeMounts))
	for mountID := range p.ownedViewTreeMounts {
		mountIDs = append(mountIDs, mountID)
	}
	p.ownedViewTreeMounts = nil
	sort.Strings(mountIDs)
	return mountIDs
}

func (p *ProtocolExtensionProcess) consumeOwnedTUIStatusKeys() []string {
	if p == nil {
		return nil
	}
	p.ownedTUIStatusMu.Lock()
	defer p.ownedTUIStatusMu.Unlock()
	keys := make([]string, 0, len(p.ownedTUIStatusKeys))
	for key := range p.ownedTUIStatusKeys {
		keys = append(keys, key)
	}
	p.ownedTUIStatusKeys = nil
	sort.Strings(keys)
	return keys
}

func (p *ProtocolExtensionProcess) consumeOwnedTUIState() struct {
	title         bool
	working       bool
	thinkingLabel bool
} {
	if p == nil {
		return struct {
			title         bool
			working       bool
			thinkingLabel bool
		}{}
	}
	p.ownedTUIStateMu.Lock()
	defer p.ownedTUIStateMu.Unlock()
	owned := struct {
		title         bool
		working       bool
		thinkingLabel bool
	}{
		title:         p.ownedTUITitle,
		working:       p.ownedTUIWorking,
		thinkingLabel: p.ownedTUIThinkingLabel,
	}
	p.ownedTUITitle = false
	p.ownedTUIWorking = false
	p.ownedTUIThinkingLabel = false
	return owned
}

func (p *ProtocolExtensionProcess) sourceInfo() ProtocolSourceInfo {
	if p == nil {
		return ProtocolSourceInfo{}
	}
	if protocolSourceInfoKey(p.Spec.Metadata) != "" {
		return p.Spec.Metadata
	}
	id := strings.TrimSpace(p.Spec.ID)
	if id == "" {
		id = "process"
	}
	return ProtocolSourceInfo{
		Path:   "<process:" + id + ">",
		Source: "process:" + id,
		Scope:  "temporary",
		Origin: "package",
	}
}

func (p *ProtocolExtensionProcess) exitBeforeHelloError(err error) error {
	message := "extension process exited before hello"
	if err != nil {
		message += ": " + err.Error()
	}
	if tail := p.stderrTailString(); tail != "" {
		message += ": " + tail
	}
	return errors.New(message)
}

func (p *ProtocolExtensionProcess) helloTimeoutError(err error) error {
	message := "extension process hello timeout"
	if err != nil {
		message += ": " + err.Error()
	}
	if tail := p.stderrTailString(); tail != "" {
		message += ": " + tail
	}
	return errors.New(message)
}

func (p *ProtocolExtensionProcess) shutdownTimeoutError(err error) error {
	message := "extension process shutdown timeout"
	if err != nil {
		message += ": " + err.Error()
	}
	if tail := p.stderrTailString(); tail != "" {
		message += ": " + tail
	}
	return errors.New(message)
}

func (p *ProtocolExtensionProcess) stderrTailString() string {
	if p != nil && p.stderrTail != nil {
		return strings.TrimSpace(p.stderrTail.String())
	}
	return ""
}

func (p *ProtocolExtensionProcess) emitExitDiagnostic(err error) {
	if p == nil {
		return
	}
	err = normalizeProcessExitError(err)
	if err == nil {
		return
	}
	runtime := protocolExtensionRuntimeFromHost(p.host)
	if runtime == nil {
		return
	}
	message := "extension process exited: " + err.Error()
	if tail := p.stderrTailString(); tail != "" {
		message += ": " + tail
	}
	runtime.emitExtensionError(ProtocolExtensionError{
		ExtensionPath: p.sourceInfo().Path,
		Event:         "process.exit",
		Error:         message,
	})
}

func protocolExtensionRuntimeFromHost(host *RPCSessionHost) *ProtocolExtensionRuntime {
	if host == nil {
		return nil
	}
	session := host.sessionSnapshot()
	if session == nil {
		return nil
	}
	return session.ExtensionRuntime
}

type processOutputTail struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func (t *processOutputTail) Write(p []byte) (int, error) {
	if t == nil {
		return len(p), nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	limit := t.limit
	if limit <= 0 {
		limit = 4096
	}
	t.buf = append(t.buf, p...)
	if len(t.buf) > limit {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-limit:]...)
	}
	return len(p), nil
}

func (t *processOutputTail) String() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

func (p *ProtocolExtensionProcess) markReady(err error) {
	if p == nil {
		return
	}
	p.readyOnce.Do(func() {
		p.readyMu.Lock()
		p.readyMarked = true
		p.readyMu.Unlock()
		p.ready <- err
		close(p.ready)
	})
}

func (p *ProtocolExtensionProcess) readyWasMarked() bool {
	if p == nil {
		return false
	}
	p.readyMu.Lock()
	defer p.readyMu.Unlock()
	return p.readyMarked
}

func (p *ProtocolExtensionProcess) writeJSON(value any) error {
	line, err := SerializeJSONLine(value)
	if err != nil {
		return err
	}
	return p.writeLine(line)
}

func (p *ProtocolExtensionProcess) writeLine(line string) error {
	if p == nil || p.stdin == nil {
		return errors.New("extension process stdin is unavailable")
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_, err := io.WriteString(p.stdin, line)
	return err
}

func (p *ProtocolExtensionProcess) kill() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	p.unsubscribeViewTreeEvents()
	err := killHostProcess(p.cmd.Process)
	p.cancelProcess()
	p.cleanupOwnedViewTreeMounts()
	p.cleanupOwnedTUIStatuses()
	p.cleanupOwnedTUIState()
	p.cleanupRuntimeRegistrations()
	return err
}

func (p *ProtocolExtensionProcess) cancelProcess() {
	if p != nil && p.cancel != nil {
		p.cancel()
	}
}

func protocolExtensionProcessEnv(spec ProtocolPackageProcessExtension) []string {
	env := os.Environ()
	for key, value := range spec.Env {
		key = strings.TrimSpace(key)
		if key == "" || strings.Contains(key, "=") || protocolExtensionProcessHostEnvKey(key) {
			continue
		}
		env = append(env, key+"="+value)
	}
	for key, value := range protocolExtensionProcessHostEnv(spec) {
		if value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func protocolExtensionProcessHostEnv(spec ProtocolPackageProcessExtension) map[string]string {
	source := (&ProtocolExtensionProcess{Spec: spec}).sourceInfo()
	return map[string]string{
		"GI_EXTENSION_ID":          spec.ID,
		"GI_EXTENSION_PATH":        source.Path,
		"GI_EXTENSION_SOURCE":      source.Source,
		"GI_EXTENSION_SCOPE":       source.Scope,
		"GI_EXTENSION_ORIGIN":      source.Origin,
		"GI_EXTENSION_PACKAGE_DIR": spec.PackageDir,
	}
}

func protocolExtensionProcessHostEnvKey(key string) bool {
	switch strings.TrimSpace(key) {
	case "GI_EXTENSION_ID",
		"GI_EXTENSION_PATH",
		"GI_EXTENSION_SOURCE",
		"GI_EXTENSION_SCOPE",
		"GI_EXTENSION_ORIGIN",
		"GI_EXTENSION_PACKAGE_DIR":
		return true
	default:
		return false
	}
}

func normalizeProcessExitError(err error) error {
	if err == nil {
		return nil
	}
	if exitError, ok := err.(*exec.ExitError); ok && exitError.ProcessState != nil && exitError.ProcessState.Success() {
		return nil
	}
	return err
}

func protocolExtensionLineType(line string) string {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := jsonUnmarshalString(line, &envelope); err != nil {
		return ""
	}
	return envelope.Type
}

func jsonUnmarshalString(line string, value any) error {
	return json.Unmarshal([]byte(line), value)
}
