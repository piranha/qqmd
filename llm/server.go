package llm

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Server manages a llama-server subprocess.
type Server struct {
	cmd     *exec.Cmd
	Port    int
	BaseURL string
	model   string
}

// ServerConfig configures a llama-server instance.
type ServerConfig struct {
	Binary    string
	Model     string
	Embedding bool   // run in embedding mode
	CtxSize   int    // context size (default 2048)
	Threads   int    // CPU threads (0 = auto)
	GPULayers int    // GPU layers (-1 = all, 0 = CPU only)
}

// StartServer starts a new llama-server subprocess and waits for it to be healthy.
func StartServer(cfg ServerConfig) (*Server, error) {
	port, err := findFreePort()
	if err != nil {
		return nil, fmt.Errorf("finding free port: %w", err)
	}

	ctxSize := cfg.CtxSize
	if ctxSize <= 0 {
		ctxSize = 2048
	}

	args := []string{
		"-m", cfg.Model,
		"--port", strconv.Itoa(port),
		"-c", strconv.Itoa(ctxSize),
		"--no-warmup",
		"--log-disable",
	}
	if cfg.Embedding {
		args = append(args, "--embedding")
	}
	if cfg.Threads > 0 {
		args = append(args, "-t", strconv.Itoa(cfg.Threads))
	}
	if cfg.GPULayers != 0 {
		args = append(args, "-ngl", strconv.Itoa(cfg.GPULayers))
	}

	cmd := exec.Command(cfg.Binary, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Set library path to the binary's directory for shared libs
	binDir := filepath.Dir(cfg.Binary)
	env := os.Environ()
	switch runtime.GOOS {
	case "linux":
		env = appendEnv(env, "LD_LIBRARY_PATH", binDir)
	case "darwin":
		env = appendEnv(env, "DYLD_LIBRARY_PATH", binDir)
	}
	cmd.Env = env
	// Set process group so we can kill the whole group
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting llama-server: %w", err)
	}

	s := &Server{
		cmd:     cmd,
		Port:    port,
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		model:   cfg.Model,
	}

	// Wait for server to become healthy
	fmt.Fprintf(os.Stderr, "Starting llama-server (port %d, model %s)...\n", port, shortModelName(cfg.Model))

	if err := s.waitHealthy(60 * time.Second); err != nil {
		s.Stop()
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "llama-server ready on port %d\n", port)
	return s, nil
}

func (s *Server) waitHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		// Check if process has died
		if s.cmd.ProcessState != nil {
			return fmt.Errorf("llama-server exited prematurely")
		}

		resp, err := client.Get(s.BaseURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("llama-server did not become healthy within %v", timeout)
}

// Stop terminates the llama-server process.
func (s *Server) Stop() {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return
	}
	// Send SIGTERM to process group
	pgid, err := syscall.Getpgid(s.cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGTERM)
	} else {
		s.cmd.Process.Signal(syscall.SIGTERM)
	}

	// Give it a moment to shut down gracefully
	done := make(chan struct{})
	go func() {
		s.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		if pgid, err := syscall.Getpgid(s.cmd.Process.Pid); err == nil {
			syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			s.cmd.Process.Kill()
		}
		<-done
	}
}

// Running returns true if the server process is still alive.
func (s *Server) Running() bool {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return false
	}
	// Try to check if the process is still running
	err := s.cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

func shortModelName(path string) string {
	base := filepath.Base(path)
	if len(base) > 40 {
		base = base[:37] + "..."
	}
	return base
}

// appendEnv adds or appends to an environment variable in an env slice.
func appendEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = e + ":" + value
			return env
		}
	}
	return append(env, prefix+value)
}
