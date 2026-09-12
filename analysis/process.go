package analysis

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultStartupTimeout  = 20 * time.Second
	defaultShutdownTimeout = 5 * time.Second
	defaultDiagnosticLimit = 64 << 10
	maxReadyMessageBytes   = 64 << 10
	tokenEnvironmentName   = "MEKING_ANALYSIS_TOKEN"
)

// ProcessConfig defines one caller-owned child process. Command and Args are
// installation concerns; protocol identity and security are not configurable.
type ProcessConfig struct {
	Command              string
	Args                 []string
	Directory            string
	Environment          []string
	StartupTimeout       time.Duration
	RequestTimeout       time.Duration
	ShutdownTimeout      time.Duration
	MaxResponseBytes     int64
	MaxDiagnosticBytes   int
	RequiredCapabilities []Capability
}

// Diagnostics reports bounded child stderr metadata without exposing its
// potentially private content to normal application results or error strings.
type Diagnostics struct {
	Bytes     int64
	Retained  int
	Truncated bool
}

// Session owns a single authenticated child process and its loopback client.
// Close first requests graceful termination, then forcefully ends the child if
// it exceeds the configured shutdown timeout.
type Session struct {
	process         *os.Process
	client          *Client
	cache           Cache
	health          Health
	wait            <-chan error
	shutdownTimeout time.Duration
	diagnostics     *boundedDiagnostics
	closeOnce       sync.Once
	closeDone       chan struct{}
	closeErr        error
}

// Start launches the configured executable, validates its stdout ready record,
// performs authenticated health negotiation, and only then returns a session.
func Start(ctx context.Context, config ProcessConfig, cache Cache) (*Session, error) {
	command := strings.TrimSpace(config.Command)
	if command == "" {
		return nil, unavailable("start", false, errors.New("analysis command is required"))
	}
	if err := validateRequiredCapabilities(config.RequiredCapabilities); err != nil {
		return nil, unavailable("start", false, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, unavailable("start", true, err)
	}
	startupTimeout := config.StartupTimeout
	if startupTimeout <= 0 {
		startupTimeout = defaultStartupTimeout
	}
	shutdownTimeout := config.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultShutdownTimeout
	}
	diagnosticLimit := config.MaxDiagnosticBytes
	if diagnosticLimit <= 0 {
		diagnosticLimit = defaultDiagnosticLimit
	}
	token, err := newBearerToken()
	if err != nil {
		return nil, unavailable("start", false, err)
	}
	child := exec.Command(command, config.Args...)
	child.Dir = config.Directory
	child.Env, err = childEnvironment(config.Environment, token)
	if err != nil {
		return nil, unavailable("start", false, err)
	}
	stdout, err := child.StdoutPipe()
	if err != nil {
		return nil, unavailable("start", false, err)
	}
	diagnostics := &boundedDiagnostics{limit: diagnosticLimit}
	child.Stderr = diagnostics
	if err := child.Start(); err != nil {
		retryable := !errors.Is(err, exec.ErrNotFound) && !errors.Is(err, os.ErrNotExist)
		return nil, unavailable("start", retryable, err)
	}
	wait := make(chan error, 1)
	go func() { wait <- child.Wait() }()
	ready := make(chan readyResult, 1)
	go readReadyMessage(stdout, ready)

	startupContext, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	var message readyMessage
	select {
	case result := <-ready:
		if result.err != nil {
			terminateProcess(child.Process, wait, shutdownTimeout)
			return nil, unavailable("ready", true, result.err)
		}
		message = result.message
	case err := <-wait:
		return nil, unavailable("ready", true, processExitCause(err))
	case <-startupContext.Done():
		terminateProcess(child.Process, wait, shutdownTimeout)
		return nil, unavailable("ready", true, startupContext.Err())
	}
	if err := validateReadyMessage(message); err != nil {
		terminateProcess(child.Process, wait, shutdownTimeout)
		return nil, unavailable("ready", false, err)
	}
	client, err := NewClient(ClientConfig{
		BaseURL: "http://127.0.0.1:" + strconv.Itoa(message.Port), BearerToken: token,
		RequestTimeout: config.RequestTimeout, MaxResponseBytes: config.MaxResponseBytes,
	})
	if err != nil {
		terminateProcess(child.Process, wait, shutdownTimeout)
		return nil, unavailable("health", false, err)
	}
	health, err := client.Health(startupContext, config.RequiredCapabilities...)
	if err != nil {
		terminateProcess(child.Process, wait, shutdownTimeout)
		return nil, err
	}
	if !sameCapabilities(message.Capabilities, health.Capabilities) {
		terminateProcess(child.Process, wait, shutdownTimeout)
		return nil, unavailable("health", false, errors.New("analysis ready and health capabilities do not match"))
	}
	client.cache = cache
	session := &Session{
		process: child.Process, client: client, cache: cache, health: health, wait: wait,
		shutdownTimeout: shutdownTimeout, diagnostics: diagnostics, closeDone: make(chan struct{}),
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
		case <-session.closeDone:
		}
	}()
	return session, nil
}

// Client returns the authenticated transport for consumer-side adapters.
func (s *Session) Client() *Client {
	if s == nil {
		return nil
	}
	return s.client
}

// Health returns the immutable handshake evidence captured at startup.
func (s *Session) Health() Health {
	if s == nil {
		return Health{}
	}
	result := s.health
	result.Capabilities = append([]Capability(nil), result.Capabilities...)
	result.Resources = append([]string(nil), result.Resources...)
	return result
}

// Diagnostics returns only bounded size metadata for child stderr.
func (s *Session) Diagnostics() Diagnostics {
	if s == nil || s.diagnostics == nil {
		return Diagnostics{}
	}
	return s.diagnostics.summary()
}

// Close is idempotent and never includes child output in its returned error.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		defer close(s.closeDone)
		defer func() {
			if s.cache != nil {
				s.closeErr = errors.Join(s.closeErr, s.cache.Close())
				s.cache = nil
			}
		}()
		if s.process == nil {
			return
		}
		if err := s.process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
			_ = s.process.Kill()
		}
		timer := time.NewTimer(s.shutdownTimeout)
		defer timer.Stop()
		select {
		case <-s.wait:
		case <-timer.C:
			if err := s.process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				s.closeErr = unavailable("stop", true, err)
				return
			}
			<-s.wait
		}
	})
	<-s.closeDone
	return s.closeErr
}

type readyResult struct {
	message readyMessage
	err     error
}

func readReadyMessage(stdout io.Reader, result chan<- readyResult) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024), maxReadyMessageBytes)
	if !scanner.Scan() {
		err := scanner.Err()
		if err == nil {
			err = errors.New("analysis process closed stdout before ready")
		}
		result <- readyResult{err: err}
		return
	}
	var message readyMessage
	if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
		result <- readyResult{err: errors.New("analysis process returned an invalid ready message")}
		return
	}
	result <- readyResult{message: message}
	for scanner.Scan() {
		// stdout is reserved for the ready record. Drain unexpected content so the
		// child cannot block, but never retain it in application diagnostics.
	}
}

func validateReadyMessage(message readyMessage) error {
	if message.Event != "ready" {
		return errors.New("analysis ready event is invalid")
	}
	if message.ContractVersion != ContractVersion {
		return errors.New("analysis ready contract version is incompatible")
	}
	if message.Port < 1 || message.Port > 65535 {
		return errors.New("analysis ready port is invalid")
	}
	if message.PID <= 0 {
		return errors.New("analysis ready process identity is invalid")
	}
	return validateRequiredCapabilities(message.Capabilities)
}

func validateRequiredCapabilities(capabilities []Capability) error {
	seen := make(map[Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if !validCapability(capability) {
			return fmt.Errorf("unknown analysis capability %q", capability)
		}
		if _, ok := seen[capability]; ok {
			return fmt.Errorf("duplicate analysis capability %q", capability)
		}
		seen[capability] = struct{}{}
	}
	return nil
}

func sameCapabilities(first, second []Capability) bool {
	if len(first) != len(second) {
		return false
	}
	counts := make(map[Capability]int, len(first))
	for _, capability := range first {
		counts[capability]++
	}
	for _, capability := range second {
		counts[capability]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func newBearerToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate analysis bearer token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func childEnvironment(additional []string, token string) ([]string, error) {
	values := make(map[string]string)
	for _, name := range []string{
		"PATH", "TMPDIR", "TEMP", "TMP", "LANG", "LC_ALL", "LC_CTYPE",
		"SYSTEMROOT", "WINDIR", "PATHEXT",
	} {
		if value, ok := os.LookupEnv(name); ok {
			values[name] = value
		}
	}
	for _, entry := range additional {
		name, value, ok := strings.Cut(entry, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.ContainsRune(name, '\x00') || strings.ContainsRune(value, '\x00') {
			return nil, errors.New("analysis child environment entry is invalid")
		}
		if name == tokenEnvironmentName || name == "PYTHONDONTWRITEBYTECODE" || sensitiveEnvironmentName(name) {
			return nil, fmt.Errorf("analysis child environment variable %q is not allowed", name)
		}
		values[name] = value
	}
	values["PYTHONDONTWRITEBYTECODE"] = "1"
	values[tokenEnvironmentName] = token
	keys := make([]string, 0, len(values))
	for name := range values {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, name := range keys {
		result = append(result, name+"="+values[name])
	}
	return result, nil
}

func sensitiveEnvironmentName(name string) bool {
	upper := strings.ToUpper(name)
	for _, fragment := range []string{
		"API_KEY", "APIKEY", "SECRET", "PASSWORD", "PASSWD", "TOKEN",
		"CREDENTIAL", "PRIVATE_KEY", "ACCESS_KEY",
	} {
		if strings.Contains(upper, fragment) {
			return true
		}
	}
	return false
}

func terminateProcess(process *os.Process, wait <-chan error, timeout time.Duration) {
	if process == nil {
		return
	}
	if err := process.Signal(os.Interrupt); err != nil {
		_ = process.Kill()
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-wait:
	case <-timer.C:
		_ = process.Kill()
		<-wait
	}
}

func processExitCause(err error) error {
	if err == nil {
		return errors.New("analysis process exited before ready")
	}
	return errors.New("analysis process exited before ready")
}

type boundedDiagnostics struct {
	mu        sync.Mutex
	limit     int
	total     int64
	retained  int
	truncated bool
	data      []byte
}

func (b *boundedDiagnostics) Write(payload []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.total += int64(len(payload))
	remaining := max(b.limit-b.retained, 0)
	retained := min(remaining, len(payload))
	b.data = append(b.data, payload[:retained]...)
	b.retained += retained
	if retained < len(payload) {
		b.truncated = true
	}
	return len(payload), nil
}

func (b *boundedDiagnostics) summary() Diagnostics {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Diagnostics{Bytes: b.total, Retained: b.retained, Truncated: b.truncated}
}
