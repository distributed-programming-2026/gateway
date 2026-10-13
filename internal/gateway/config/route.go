package config

func compile(r *Route, c *Config) error {
	for _, step := range []func(*Route) error{compilePatterns, compileAction, compileMatches, compileHeaders, compileBasicAuth} {
		if err := step(r); err != nil {
			return err
		}
	}
	return compileInvocations(r, c)
}
