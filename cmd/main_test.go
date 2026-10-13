package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

type CommandSuite struct {
	suite.Suite
	path   string
	output bytes.Buffer
}

func TestCommandSuite(t *testing.T) { suite.Run(t, new(CommandSuite)) }
func (s *CommandSuite) SetupTest() {
	s.path = filepath.Join(s.T().TempDir(), "gateway.yaml")
	s.Require().NoError(os.WriteFile(s.path, []byte("kind: gateway/v1\n"), 0o600))
	s.output.Reset()
}
func (s *CommandSuite) invoke(args ...string) error {
	cmd := command()
	cmd.Writer = &s.output
	cmd.ErrWriter = &s.output
	return cmd.Run(context.Background(), append([]string{"gateway"}, args...))
}
func (s *CommandSuite) TestCheckCommands() {
	for _, args := range [][]string{
		{configFlag, s.path, "--check-config"},
		{configFlag, s.path, "check-config"},
		{"service", configFlag, s.path, "--check-config"},
	} {
		s.output.Reset()
		s.Require().NoError(s.invoke(args...))
		s.Contains(s.output.String(), "configuration valid")
	}
}
func (s *CommandSuite) TestConfigFromEnvironment() {
	s.T().Setenv("GATEWAY_CONFIG", s.path)
	s.Require().NoError(s.invoke("check-config"))
	s.Contains(s.output.String(), "configuration valid")
}
func (s *CommandSuite) TestInvalidConfigAndArguments() {
	s.Error(s.invoke(configFlag, s.path+".missing", "check-config"))
	s.Error(s.invoke("--unknown-flag"))
	s.Require().NoError(os.WriteFile(s.path, []byte("kind: gateway/v1\nunknown: true\n"), 0o600))
	s.Error(s.invoke(configFlag, s.path, "check-config"))
}
func (s *CommandSuite) TestHelpAndAddressFallback() {
	s.Require().NoError(s.invoke("--help"))
	s.Contains(s.output.String(), "service")
	s.Contains(s.output.String(), "check-config")
	s.Equal(":8000", envDefault("GATEWAY_TEST_ADDRESS", ":8000"))
	s.T().Setenv("GATEWAY_TEST_ADDRESS", "127.0.0.1:1234")
	s.Equal("127.0.0.1:1234", envDefault("GATEWAY_TEST_ADDRESS", ":8000"))
}

const configFlag = "--config"
