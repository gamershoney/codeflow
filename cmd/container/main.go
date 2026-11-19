package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type containerConfig struct {
	codeflowDomain string
	pocketDomain   string
	stateRoot      string
	repositoryRoot string
	caddyfile      string
	encryptionKey  string
	encryptionFile string
}

type childProcess struct {
	name    string
	command *exec.Cmd
}

type childResult struct {
	name string
	err  error
}

func main() {
	config, err := loadContainerConfig()
	if err != nil {
		log.Fatal(err)
	}
	if err := config.prepare(); err != nil {
		log.Fatal(err)
	}
	if err := config.applyEnvironment(); err != nil {
		log.Fatal(err)
	}

	children := []childProcess{
		{name: "pocket-id", command: command("/usr/local/bin/pocket-id", filepath.Join(config.stateRoot, "pocket-id"))},
		{name: "codeflow", command: command("/usr/local/bin/codeflow-server", "/app")},
		{name: "caddy", command: command("/usr/bin/caddy", "/app", "run", "--config", config.caddyfile, "--adapter", "caddyfile")},
	}
	results := make(chan childResult, len(children))
	started := make([]childProcess, 0, len(children))
	for _, child := range children {
		log.Printf("starting %s", child.name)
		if err := child.command.Start(); err != nil {
			terminate(started)
			log.Fatalf("start %s: %v", child.name, err)
		}
		started = append(started, child)
		go func(child childProcess) { results <- childResult{name: child.name, err: child.command.Wait()} }(child)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.Signal(15))
	var first childResult
	select {
	case received := <-signals:
		first = childResult{name: "signal", err: fmt.Errorf("received %s", received)}
		log.Printf("shutting down: %v", first.err)
	case first = <-results:
		log.Printf("%s exited: %v", first.name, first.err)
	}
	terminate(started)

	remaining := len(started)
	if first.name != "signal" {
		remaining--
	}
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	for remaining > 0 {
		select {
		case <-results:
			remaining--
		case <-timer.C:
			for _, child := range started {
				if child.command.Process != nil {
					_ = child.command.Process.Kill()
				}
			}
			remaining = 0
		}
	}
	if first.name != "signal" {
		os.Exit(1)
	}
}

func loadContainerConfig() (containerConfig, error) {
	config := containerConfig{
		codeflowDomain: strings.TrimSpace(os.Getenv("CODEFLOW_DOMAIN")),
		pocketDomain:   strings.TrimSpace(os.Getenv("POCKET_ID_DOMAIN")),
		stateRoot:      valueOrDefault("CODEFLOW_STATE_ROOT", "/data"),
		repositoryRoot: valueOrDefault("CODEFLOW_REPOSITORIES_PATH", "/repos"),
		caddyfile:      valueOrDefault("CODEFLOW_CADDYFILE", "/etc/caddy/Caddyfile"),
		encryptionKey:  os.Getenv("POCKET_ID_ENCRYPTION_KEY"),
		encryptionFile: strings.TrimSpace(os.Getenv("POCKET_ID_ENCRYPTION_KEY_FILE")),
	}
	if err := validateDomain("CODEFLOW_DOMAIN", config.codeflowDomain); err != nil {
		return containerConfig{}, err
	}
	if err := validateDomain("POCKET_ID_DOMAIN", config.pocketDomain); err != nil {
		return containerConfig{}, err
	}
	if strings.EqualFold(config.codeflowDomain, config.pocketDomain) {
		return containerConfig{}, errors.New("CODEFLOW_DOMAIN and POCKET_ID_DOMAIN must be different hostnames")
	}
	if len(config.encryptionKey) < 16 && config.encryptionFile == "" {
		return containerConfig{}, errors.New("set POCKET_ID_ENCRYPTION_KEY to at least 16 characters or provide POCKET_ID_ENCRYPTION_KEY_FILE")
	}
	return config, nil
}

func validateDomain(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	parsed, err := url.Parse("https://" + value)
	if err != nil || parsed.Hostname() == "" || parsed.Host != value || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must be a hostname without a scheme or path", name)
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && !ip.IsLoopback() {
		return fmt.Errorf("%s must be a DNS hostname so HTTPS certificates and passkeys work", name)
	}
	return nil
}

func (config containerConfig) prepare() error {
	stateDirectories := []string{
		filepath.Join(config.stateRoot, "codeflow"),
		filepath.Join(config.stateRoot, "pocket-id"),
		filepath.Join(config.stateRoot, "caddy-data"),
		filepath.Join(config.stateRoot, "caddy-config"),
		filepath.Join(config.stateRoot, "home"),
	}
	for _, directory := range stateDirectories {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", directory, err)
		}
		if runtime.GOOS == "linux" && os.Geteuid() == 0 {
			if err := os.Chown(directory, 1000, 1000); err != nil {
				return fmt.Errorf("set ownership on %s: %w", directory, err)
			}
		}
	}
	if err := os.MkdirAll(config.repositoryRoot, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", config.repositoryRoot, err)
	}
	return nil
}

func (config containerConfig) applyEnvironment() error {
	values := map[string]string{
		"CODEFLOW_ADDR":                  "127.0.0.1:8787",
		"CODEFLOW_DATA":                  filepath.Join(config.stateRoot, "codeflow", "project.json"),
		"CODEFLOW_DISCOVERY_ROOTS":       config.repositoryRoot,
		"CODEFLOW_MCP_RESOURCE":          "https://" + config.codeflowDomain,
		"CODEFLOW_MCP_OAUTH_ISSUER":      "https://" + config.pocketDomain,
		"CODEFLOW_MCP_OAUTH_JWKS_URL":    "http://127.0.0.1:1411/.well-known/jwks.json",
		"APP_URL":                        "https://" + config.pocketDomain,
		"APP_ENV":                        "production",
		"DB_CONNECTION_STRING":           filepath.Join(config.stateRoot, "pocket-id", "pocket-id.db"),
		"HOME":                           filepath.Join(config.stateRoot, "home"),
		"HOST":                           "127.0.0.1",
		"PORT":                           "1411",
		"TRUST_PROXY":                    "127.0.0.1/32,::1/128",
		"ALLOW_INSECURE_CALLBACK_URLS":   "false",
		"XDG_DATA_HOME":                  filepath.Join(config.stateRoot, "caddy-data"),
		"XDG_CONFIG_HOME":                filepath.Join(config.stateRoot, "caddy-config"),
		"VERSION_CHECK_DISABLED":         "true",
		"ANALYTICS_DISABLED":             "true",
		"ICON_LIBRARY_URL":               "disabled",
		"CODEFLOW_MCP_OAUTH_READ_SCOPE":  defaultReadScope,
		"CODEFLOW_MCP_OAUTH_WRITE_SCOPE": defaultWriteScope,
	}
	if config.encryptionFile != "" {
		values["ENCRYPTION_KEY_FILE"] = config.encryptionFile
	} else {
		values["ENCRYPTION_KEY"] = config.encryptionKey
	}
	for key, value := range values {
		if existing := os.Getenv(key); existing != "" && allowsOverride(key) {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	return nil
}

const (
	defaultReadScope  = "codeflow:read"
	defaultWriteScope = "codeflow:write"
)

func allowsOverride(key string) bool {
	switch key {
	case "CODEFLOW_DISCOVERY_ROOTS", "CODEFLOW_MCP_OAUTH_READ_SCOPE", "CODEFLOW_MCP_OAUTH_WRITE_SCOPE", "VERSION_CHECK_DISABLED", "ANALYTICS_DISABLED", "ICON_LIBRARY_URL":
		return true
	default:
		return false
	}
}

func valueOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func command(path, directory string, args ...string) *exec.Cmd {
	commandPath := path
	commandArgs := args
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		commandPath = "/sbin/su-exec"
		commandArgs = append([]string{"1000:1000", path}, args...)
	}
	cmd := exec.Command(commandPath, commandArgs...)
	cmd.Dir = directory
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func terminate(children []childProcess) {
	for _, child := range children {
		if child.command.Process != nil {
			_ = child.command.Process.Signal(syscall.Signal(15))
		}
	}
}
