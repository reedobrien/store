// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"fmt"
	"strings"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/environs/sync"
	envtesting "launchpad.net/juju-core/environs/testing"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/juju/arch"
	"launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/version"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

const (
	useDefaultKeys = true
	noKeysDefined  = false
)

type bootstrapSuite struct {
	home *coretesting.FakeHome
	testbase.LoggingSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.home = coretesting.MakeFakeHomeNoEnvironments(c, "foo")
}

func (s *bootstrapSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
	s.ToolsFixture.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *bootstrapSuite) TestBootstrapNeedsSettings(c *gc.C) {
	env := newEnviron("bar", noKeysDefined, nil)
	s.setDummyStorage(c, env)
	fixEnv := func(key string, value interface{}) {
		cfg, err := env.Config().Apply(map[string]interface{}{
			key: value,
		})
		c.Assert(err, gc.IsNil)
		env.cfg = cfg
	}

	err := bootstrap.Bootstrap(coretesting.Context(c), env, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "environment configuration has no admin-secret")

	fixEnv("admin-secret", "whatever")
	err = bootstrap.Bootstrap(coretesting.Context(c), env, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "environment configuration has no ca-cert")

	fixEnv("ca-cert", coretesting.CACert)
	err = bootstrap.Bootstrap(coretesting.Context(c), env, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "environment configuration has no ca-private-key")

	fixEnv("ca-private-key", coretesting.CAKey)
	uploadTools(c, env)
	err = bootstrap.Bootstrap(coretesting.Context(c), env, constraints.Value{})
	c.Assert(err, gc.IsNil)
}

func uploadTools(c *gc.C, env environs.Environ) {
	usefulVersion := version.Current
	usefulVersion.Series = env.Config().DefaultSeries()
	envtesting.AssertUploadFakeToolsVersions(c, env.Storage(), usefulVersion)
}

func (s *bootstrapSuite) TestBootstrapEmptyConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err := bootstrap.Bootstrap(coretesting.Context(c), env, constraints.Value{})
	c.Assert(err, gc.IsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.constraints, gc.DeepEquals, constraints.Value{})
}

func (s *bootstrapSuite) TestBootstrapSpecifiedConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	cons := constraints.MustParse("cpu-cores=2 mem=4G")
	err := bootstrap.Bootstrap(coretesting.Context(c), env, cons)
	c.Assert(err, gc.IsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.constraints, gc.DeepEquals, cons)
}

var bootstrapSetAgentVersionTests = []envtesting.BootstrapToolsTest{
	{
		Info:          "released cli with dev setting picks newest matching 1",
		Available:     envtesting.V100Xall,
		CliVersion:    envtesting.V100q32,
		DefaultSeries: "precise",
		Development:   true,
		Expect:        []version.Binary{envtesting.V1001p64},
	}, {
		Info:          "released cli with dev setting picks newest matching 2",
		Available:     envtesting.V1all,
		CliVersion:    envtesting.V120q64,
		DefaultSeries: "precise",
		Development:   true,
		Arch:          "i386",
		Expect:        []version.Binary{envtesting.V120p32},
	}, {
		Info:          "dev cli picks newest matching 1",
		Available:     envtesting.V110Xall,
		CliVersion:    envtesting.V110q32,
		DefaultSeries: "precise",
		Expect:        []version.Binary{envtesting.V1101p64},
	}, {
		Info:          "dev cli picks newest matching 2",
		Available:     envtesting.V1all,
		CliVersion:    envtesting.V120q64,
		DefaultSeries: "precise",
		Arch:          "i386",
		Expect:        []version.Binary{envtesting.V120p32},
	}, {
		Info:          "dev cli has different arch to available",
		Available:     envtesting.V1all,
		CliVersion:    envtesting.V310qppc64,
		DefaultSeries: "precise",
		Expect:        []version.Binary{envtesting.V3101qppc64},
	}}

func (s *bootstrapSuite) TestBootstrapTools(c *gc.C) {
	allTests := append(envtesting.BootstrapToolsTests, bootstrapSetAgentVersionTests...)
	// version.Current is set in the loop so ensure it is restored later.
	s.PatchValue(&version.Current, version.Current)
	for i, test := range allTests {
		c.Logf("\ntest %d: %s", i, test.Info)
		dummy.Reset()
		attrs := dummy.SampleConfig().Merge(coretesting.Attrs{
			"state-server":   false,
			"development":    test.Development,
			"default-series": test.DefaultSeries,
		})
		if test.AgentVersion != version.Zero {
			attrs["agent-version"] = test.AgentVersion.String()
		}
		cfg, err := config.New(config.NoDefaults, attrs)
		c.Assert(err, gc.IsNil)
		env, err := environs.Prepare(cfg, coretesting.Context(c), configstore.NewMem())
		c.Assert(err, gc.IsNil)
		envtesting.RemoveAllTools(c, env)

		version.Current = test.CliVersion
		envtesting.AssertUploadFakeToolsVersions(c, env.Storage(), test.Available...)
		// Remove the default tools URL from the search path, just look in cloud storage.
		s.PatchValue(&envtools.DefaultBaseURL, "")

		cons := constraints.Value{}
		if test.Arch != "" {
			cons = constraints.MustParse("arch=" + test.Arch)
		}
		err = bootstrap.Bootstrap(coretesting.Context(c), env, cons)
		if test.Err != "" {
			c.Check(err, gc.NotNil)
			if err != nil {
				stripped := strings.Replace(err.Error(), "\n", "", -1)
				c.Check(stripped, gc.Matches, ".*"+stripped)
			}
			continue
		} else {
			c.Check(err, gc.IsNil)
		}
		unique := map[version.Number]bool{}
		for _, expected := range test.Expect {
			unique[expected.Number] = true
		}
		for expectAgentVersion := range unique {
			agentVersion, ok := env.Config().AgentVersion()
			c.Check(ok, gc.Equals, true)
			c.Check(agentVersion, gc.Equals, expectAgentVersion)
		}
	}
}

func (s *bootstrapSuite) TestBootstrapNoTools(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	envtesting.RemoveFakeTools(c, env.Storage())
	err := bootstrap.Bootstrap(coretesting.Context(c), env, constraints.Value{})
	// bootstrap.Bootstrap leaves it to the provider to
	// locate bootstrap tools.
	c.Assert(err, gc.IsNil)
}

func (s *bootstrapSuite) TestEnsureToolsAvailabilityIncompatibleHostArch(c *gc.C) {
	// Host runs amd64, want ppc64 tools.
	s.PatchValue(&arch.HostArch, func() string {
		return "amd64"
	})
	// Fake a dev version so tools can be automatically uploaded.
	devVersion := version.Current
	devVersion.Minor = 11
	s.PatchValue(&version.Current, devVersion)
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	envtesting.RemoveFakeTools(c, env.Storage())
	arch := "ppc64"
	_, err := bootstrap.EnsureToolsAvailability(coretesting.Context(c), env, env.Config().DefaultSeries(), &arch)
	c.Assert(err, gc.NotNil)
	stripped := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(stripped,
		gc.Matches,
		`cannot upload bootstrap tools: cannot build tools for "ppc64" using a machine running on "amd64"`)
}

func (s *bootstrapSuite) TestEnsureToolsAvailabilityIncompatibleTargetArch(c *gc.C) {
	// Host runs ppc64, environment only supports amd64, arm64.
	s.PatchValue(&arch.HostArch, func() string {
		return "ppc64"
	})
	// Fake a dev version so tools can be automatically uploaded.
	devVersion := version.Current
	devVersion.Minor = 11
	s.PatchValue(&version.Current, devVersion)
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	envtesting.RemoveFakeTools(c, env.Storage())
	_, err := bootstrap.EnsureToolsAvailability(coretesting.Context(c), env, env.Config().DefaultSeries(), nil)
	c.Assert(err, gc.NotNil)
	stripped := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(stripped,
		gc.Matches,
		`cannot upload bootstrap tools: environment "foo" of type dummy does not support instances running on "ppc64"`)
}

func (s *bootstrapSuite) TestEnsureToolsAvailabilityAgentVersionAlreadySet(c *gc.C) {
	// Can't upload tools if agent version already set.
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{"agent-version": "1.16.0"})
	s.setDummyStorage(c, env)
	envtesting.RemoveFakeTools(c, env.Storage())
	_, err := bootstrap.EnsureToolsAvailability(coretesting.Context(c), env, env.Config().DefaultSeries(), nil)
	c.Assert(err, gc.NotNil)
	stripped := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(stripped,
		gc.Matches,
		"cannot upload bootstrap tools: Juju cannot bootstrap because no tools are available for your environment.*")
}

// getMockBuildTools returns a sync.BuildToolsTarballFunc implementation which generates
// a fake tools tarball.
func (s *bootstrapSuite) getMockBuildTools(c *gc.C) sync.BuildToolsTarballFunc {
	toolsDir := c.MkDir()
	return func(forceVersion *version.Number) (*sync.BuiltTools, error) {
		// UploadFakeToolsVersions requires a storage to write to.
		stor, err := filestorage.NewFileStorageWriter(toolsDir)
		c.Assert(err, gc.IsNil)
		vers := version.Current
		if forceVersion != nil {
			vers.Number = *forceVersion
		}
		versions := []version.Binary{vers}
		uploadedTools, err := envtesting.UploadFakeToolsVersions(stor, versions...)
		c.Assert(err, gc.IsNil)
		agentTools := uploadedTools[0]
		return &sync.BuiltTools{
			Dir:         toolsDir,
			StorageName: envtools.StorageName(vers),
			Version:     vers,
			Size:        agentTools.Size,
			Sha256Hash:  agentTools.SHA256,
		}, nil
	}
}

func (s *bootstrapSuite) TestEnsureToolsAvailability(c *gc.C) {
	existingToolsVersion := version.MustParseBinary("1.19.0-trusty-amd64")
	s.PatchValue(&version.Current, existingToolsVersion)
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	// At this point, as a result of setDummyStorage, env has tools for amd64 uploaded.
	// Set version.Current to be arm64 to simulate a different CLI version.
	cliVersion := version.Current
	cliVersion.Arch = "arm64"
	version.Current = cliVersion
	s.PatchValue(&sync.BuildToolsTarball, s.getMockBuildTools(c))
	// Host runs arm64, environment supports arm64.
	s.PatchValue(&arch.HostArch, func() string {
		return "arm64"
	})
	arch := "arm64"
	agentTools, err := bootstrap.EnsureToolsAvailability(coretesting.Context(c), env, env.Config().DefaultSeries(), &arch)
	c.Assert(err, gc.IsNil)
	c.Assert(agentTools, gc.HasLen, 1)
	expectedVers := version.Current
	expectedVers.Number.Build++
	expectedVers.Series = env.Config().DefaultSeries()
	c.Assert(agentTools[0].Version, gc.DeepEquals, expectedVers)
}

func (s *bootstrapSuite) TestSeriesToUpload(c *gc.C) {
	vers := version.Current
	vers.Series = "quantal"
	s.PatchValue(&version.Current, vers)
	env := newEnviron("foo", useDefaultKeys, nil)
	cfg := env.Config()
	c.Assert(bootstrap.SeriesToUpload(cfg, nil), jc.SameContents, []string{"quantal", "precise"})
	c.Assert(bootstrap.SeriesToUpload(cfg, []string{"quantal"}), jc.SameContents, []string{"quantal"})
	env = newEnviron("foo", useDefaultKeys, map[string]interface{}{"default-series": "lucid"})
	cfg = env.Config()
	c.Assert(bootstrap.SeriesToUpload(cfg, nil), jc.SameContents, []string{"quantal", "precise", "lucid"})
}

func (s *bootstrapSuite) assertUploadTools(c *gc.C, vers version.Binary, forceVersion bool,
	extraConfig map[string]interface{}, errMessage string) {

	s.PatchValue(&version.Current, vers)
	// If we allow released tools to be uploaded, the build number is incremented so in that case
	// we need to ensure the environment is set up to allow dev tools to be used.
	env := newEnviron("foo", useDefaultKeys, extraConfig)
	s.setDummyStorage(c, env)
	envtesting.RemoveFakeTools(c, env.Storage())

	// At this point, as a result of setDummyStorage, env has tools for amd64 uploaded.
	// Set version.Current to be arm64 to simulate a different CLI version.
	cliVersion := version.Current
	cliVersion.Arch = "arm64"
	version.Current = cliVersion
	s.PatchValue(&sync.BuildToolsTarball, s.getMockBuildTools(c))
	// Host runs arm64, environment supports arm64.
	s.PatchValue(&arch.HostArch, func() string {
		return "arm64"
	})
	arch := "arm64"
	err := bootstrap.UploadTools(coretesting.Context(c), env, &arch, forceVersion, "precise")
	if errMessage != "" {
		c.Assert(err, gc.NotNil)
		stripped := strings.Replace(err.Error(), "\n", "", -1)
		c.Assert(stripped, gc.Matches, errMessage)
		return
	}
	c.Assert(err, gc.IsNil)
	params := envtools.BootstrapToolsParams{
		Arch:   &arch,
		Series: version.Current.Series,
	}
	agentTools, err := envtools.FindBootstrapTools(env, params)
	c.Assert(err, gc.IsNil)
	c.Assert(agentTools, gc.HasLen, 1)
	expectedVers := vers
	expectedVers.Number.Build++
	expectedVers.Series = version.Current.Series
	c.Assert(agentTools[0].Version, gc.DeepEquals, expectedVers)
}

func (s *bootstrapSuite) TestUploadTools(c *gc.C) {
	vers := version.MustParseBinary("1.19.0-trusty-arm64")
	s.assertUploadTools(c, vers, false, nil, "")
}

func (s *bootstrapSuite) TestUploadToolsReleaseToolsWithDevConfig(c *gc.C) {
	vers := version.MustParseBinary("1.18.0-trusty-arm64")
	extraCfg := map[string]interface{}{"development": true}
	s.assertUploadTools(c, vers, false, extraCfg, "")
}

func (s *bootstrapSuite) TestUploadToolsForceVersionAllowsAgentVersionSet(c *gc.C) {
	vers := version.MustParseBinary("1.18.0-trusty-arm64")
	extraCfg := map[string]interface{}{"agent-version": "1.18.0", "development": true}
	s.assertUploadTools(c, vers, true, extraCfg, "")
}

type bootstrapEnviron struct {
	name             string
	cfg              *config.Config
	environs.Environ // stub out all methods we don't care about.

	// The following fields are filled in when Bootstrap is called.
	bootstrapCount int
	constraints    constraints.Value
	storage        storage.Storage
}

var _ envtools.SupportsCustomSources = (*bootstrapEnviron)(nil)

// GetToolsSources returns a list of sources which are used to search for simplestreams tools metadata.
func (e *bootstrapEnviron) GetToolsSources() ([]simplestreams.DataSource, error) {
	// Add the simplestreams source off the control bucket.
	return []simplestreams.DataSource{
		storage.NewStorageSimpleStreamsDataSource("cloud storage", e.Storage(), storage.BaseToolsPath)}, nil
}

func newEnviron(name string, defaultKeys bool, extraAttrs map[string]interface{}) *bootstrapEnviron {
	m := dummy.SampleConfig().Merge(extraAttrs)
	if !defaultKeys {
		m = m.Delete(
			"ca-cert",
			"ca-private-key",
			"admin-secret",
		)
	}
	cfg, err := config.New(config.NoDefaults, m)
	if err != nil {
		panic(fmt.Errorf("cannot create config from %#v: %v", m, err))
	}
	return &bootstrapEnviron{
		name: name,
		cfg:  cfg,
	}
}

// setDummyStorage injects the local provider's fake storage implementation
// into the given environment, so that tests can manipulate storage as if it
// were real.
func (s *bootstrapSuite) setDummyStorage(c *gc.C, env *bootstrapEnviron) {
	closer, stor, _ := envtesting.CreateLocalTestStorage(c)
	env.storage = stor
	envtesting.UploadFakeTools(c, env.storage)
	s.AddCleanup(func(c *gc.C) { closer.Close() })
}

func (e *bootstrapEnviron) Name() string {
	return e.name
}

func (e *bootstrapEnviron) Bootstrap(ctx environs.BootstrapContext, cons constraints.Value) error {
	e.bootstrapCount++
	e.constraints = cons
	return nil
}

func (e *bootstrapEnviron) Config() *config.Config {
	return e.cfg
}

func (e *bootstrapEnviron) SetConfig(cfg *config.Config) error {
	e.cfg = cfg
	return nil
}

func (e *bootstrapEnviron) Storage() storage.Storage {
	return e.storage
}

func (e *bootstrapEnviron) SupportedArchitectures() ([]string, error) {
	return []string{"amd64", "arm64"}, nil
}
