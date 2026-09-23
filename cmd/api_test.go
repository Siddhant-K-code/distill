package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestRenderStartContract(t *testing.T) {
	contract := loadRenderServiceContract(t)
	buildFields := strings.Fields(contract.buildCommand)
	if len(buildFields) != 5 || buildFields[0] != "go" || buildFields[1] != "build" ||
		buildFields[2] != "-o" || buildFields[4] != "." {
		t.Fatalf("unsupported Render build command %q", contract.buildCommand)
	}

	startFields := strings.Fields(contract.startCommand)
	if len(startFields) == 0 {
		t.Fatal("Render start command is empty")
	}
	if strings.TrimPrefix(startFields[0], "./") != buildFields[3] {
		t.Fatalf("start binary %q does not match built artifact %q", startFields[0], buildFields[3])
	}
	if len(startFields) < 2 || startFields[1] != "api" {
		t.Fatalf("Render start command does not select the API server: %q", contract.startCommand)
	}
	for _, field := range startFields {
		if field == "--memory" || field == "--session" {
			t.Fatalf("Render start command enables unauthenticated stateful routes: %q", contract.startCommand)
		}
	}

	tempDir := t.TempDir()
	binary := filepath.Join(tempDir, buildFields[3])
	buildFields[3] = binary
	build := exec.Command(buildFields[0], buildFields[1:]...)
	build.Dir = ".."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("run Render build command: %v\n%s", err, output)
	}

	bareOutput, err := exec.Command(binary).CombinedOutput()
	if err != nil {
		t.Fatalf("bare distill-api exited with an error: %v\n%s", err, bareOutput)
	}
	if !bytes.Contains(bareOutput, []byte("Usage:\n  distill [command]")) {
		t.Fatalf("bare distill-api did not print CLI help:\n%s", bareOutput)
	}

	port := availableTCPPort(t)
	for index, field := range startFields {
		startFields[index] = strings.ReplaceAll(field, "$PORT", strconv.Itoa(port))
	}
	startFields[0] = binary
	command := exec.Command(startFields[0], startFields[1:]...)
	command.Dir = tempDir
	command.Env = append(os.Environ(), "PORT="+strconv.Itoa(port))
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatalf("start distill-api: %v", err)
	}

	wait := make(chan error, 1)
	go func() {
		wait <- command.Wait()
	}()
	exited := false
	t.Cleanup(func() {
		if exited {
			return
		}
		_ = command.Process.Signal(syscall.SIGTERM)
		select {
		case <-wait:
		case <-time.After(5 * time.Second):
			_ = command.Process.Kill()
			<-wait
		}
	})

	healthURL := fmt.Sprintf("http://127.0.0.1:%d%s", port, contract.healthCheckPath)
	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case err := <-wait:
			exited = true
			t.Fatalf("distill-api exited before becoming healthy: %v\n%s", err, output.String())
		default:
		}

		response, requestErr := client.Get(healthURL)
		if requestErr == nil {
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr != nil {
				t.Fatalf("read health response: %v", readErr)
			}
			if response.StatusCode != http.StatusOK {
				t.Fatalf("health status = %d, want 200; body: %s", response.StatusCode, body)
			}
			if strings.TrimSpace(string(body)) != `{"status":"ok"}` {
				t.Fatalf("health body = %q, want status ok", body)
			}
			break
		}
		if time.Now().After(deadline) {
			_ = command.Process.Signal(syscall.SIGTERM)
			<-wait
			exited = true
			t.Fatalf("distill-api did not become healthy at %s: %v\n%s", healthURL, requestErr, output.String())
		}
		time.Sleep(100 * time.Millisecond)
	}

	for _, path := range []string{"/v1/memory/recall", "/v1/session/get"} {
		response, err := client.Post(
			fmt.Sprintf("http://127.0.0.1:%d%s", port, path),
			"application/json",
			strings.NewReader("{}"),
		)
		if err != nil {
			t.Fatalf("request disabled route %s: %v", path, err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("disabled route %s status = %d, want 404", path, response.StatusCode)
		}
	}

	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("stop distill-api: %v", err)
	}
	if err := <-wait; err != nil {
		t.Fatalf("distill-api shutdown: %v\n%s", err, output.String())
	}
	exited = true
	if !strings.Contains(output.String(), "Memory: false\n  Sessions: false") {
		t.Fatalf("Render command unexpectedly enabled stateful routes:\n%s", output.String())
	}
}

func TestResolveAPIListenAddressUsesConfigFallbacks(t *testing.T) {
	t.Setenv("PORT", "")
	command := newAPIListenAddressTestCommand()

	host, port, err := resolveAPIListenAddress(command, "127.0.0.1", 9000)
	if err != nil {
		t.Fatal(err)
	}

	if host != "127.0.0.1" {
		t.Fatalf("host = %q, want config fallback", host)
	}
	if port != 9000 {
		t.Fatalf("port = %d, want config fallback", port)
	}
}

func TestResolveAPIListenAddressPrefersExplicitFlags(t *testing.T) {
	t.Setenv("PORT", "19090")
	command := newAPIListenAddressTestCommand()
	if err := command.Flags().Set("host", "0.0.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := command.Flags().Set("port", "18080"); err != nil {
		t.Fatal(err)
	}

	host, port, err := resolveAPIListenAddress(command, "127.0.0.1", 9000)
	if err != nil {
		t.Fatal(err)
	}

	if host != "0.0.0.0" {
		t.Fatalf("host = %q, want explicit flag", host)
	}
	if port != 18080 {
		t.Fatalf("port = %d, want explicit flag", port)
	}
}

func TestResolveAPIListenAddressUsesRenderPortAndPreservesHost(t *testing.T) {
	t.Setenv("PORT", "18081")
	command := newAPIListenAddressTestCommand()

	host, port, err := resolveAPIListenAddress(command, "127.0.0.1", 9000)
	if err != nil {
		t.Fatal(err)
	}

	if host != "127.0.0.1" {
		t.Fatalf("host = %q, want configured host preserved", host)
	}
	if port != 18081 {
		t.Fatalf("port = %d, want Render PORT", port)
	}
}

func TestResolveAPIListenAddressRejectsInvalidRenderPort(t *testing.T) {
	t.Setenv("PORT", "not-a-port")
	command := newAPIListenAddressTestCommand()

	if _, _, err := resolveAPIListenAddress(command, "127.0.0.1", 9000); err == nil {
		t.Fatal("invalid Render PORT unexpectedly accepted")
	}
}

func newAPIListenAddressTestCommand() *cobra.Command {
	command := &cobra.Command{}
	command.Flags().String("host", "0.0.0.0", "")
	command.Flags().Int("port", 8080, "")
	return command
}

func availableTCPPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find available TCP port: %v", err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			t.Errorf("close port probe: %v", err)
		}
	}()

	return listener.Addr().(*net.TCPAddr).Port
}

type renderServiceContract struct {
	buildCommand    string
	startCommand    string
	healthCheckPath string
}

func loadRenderServiceContract(t *testing.T) renderServiceContract {
	t.Helper()

	content, err := os.ReadFile(filepath.Join("..", "render.yaml"))
	if err != nil {
		t.Fatalf("read render.yaml: %v", err)
	}

	var contract renderServiceContract
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "buildCommand:"):
			contract.buildCommand = strings.TrimSpace(strings.TrimPrefix(line, "buildCommand:"))
		case strings.HasPrefix(line, "startCommand:"):
			contract.startCommand = strings.TrimSpace(strings.TrimPrefix(line, "startCommand:"))
		case strings.HasPrefix(line, "healthCheckPath:"):
			contract.healthCheckPath = strings.TrimSpace(strings.TrimPrefix(line, "healthCheckPath:"))
		}
	}

	if contract.buildCommand == "" || contract.startCommand == "" || contract.healthCheckPath == "" {
		t.Fatalf("render.yaml is missing its build, start, or health-check contract")
	}
	return contract
}
