package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ConfigSuite struct {
	suite.Suite
	dir string
}

func file(t *testing.T, dir, name, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
}
func (s *ConfigSuite) SetupTest()  { s.dir = s.T().TempDir() }
func TestConfigSuite(t *testing.T) { suite.Run(t, new(ConfigSuite)) }
