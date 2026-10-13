package integration_tests

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExecutableStartsWithYAMLAndStopsOnSignal(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "gateway")
	build := exec.Command("go", "build", "-o", binary, "./cmd")
	build.Dir = "../.."
	{
		output, err := build.CombinedOutput()
		require.NoError(t, err,
			"build executable: %v\n%s", err, output)
	}

	conf := filepath.Join(dir, "gateway.yaml")
	require.NoError(t, os.WriteFile(conf, []byte("kind: gateway/v1\ngatewayHealthHost: 0\n---\nkind: gateway/service/v1\nname: local\nroutes: [{pattern: '^/', directResponse: {status: 200, inlineString: running}}]\n"), 0o600))

	// Reserve addresses briefly; the executable owns the final binds.
	reserve := func() string {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		addr := l.Addr().String()
		_ = l.Close()
		return addr
	}
	proxy := reserve()
	health := reserve()
	cmd := exec.Command(binary, "--config", conf)
	cmd.Env = append(os.Environ(), "GATEWAY_HTTP_ADDRESS="+proxy, "GATEWAY_HEALTH_ADDRESS="+health)
	var logs bytes.Buffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	require.NoError(t, cmd.Start())

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	client := &http.Client{Timeout: time.Second}
	waitStatus(t, client, "http://"+health+"/readyz", 204)
	response, err := client.Get("http://" + proxy + "/test")
	require.NoError(t, err)

	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	require.NoError(t, err,
		"executable did not proxy/configure correctly")
	require.Equal(t, "running", string(body),
		"executable did not proxy/configure correctly")
	err = cmd.Process.Signal(syscall.SIGTERM)
	require.NoError(t, err)

	select {
	case err = <-done:
		require.NoError(t, err,
			"process failed: %v %s", err, logs.String())

	case <-time.After(3 * time.Second):
		require.FailNow(t, "process did not stop")
	}
}
