// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"errors"
	"os/user"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container/kvm"
	lxctesting "launchpad.net/juju-core/container/lxc/testing"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/provider/local"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type baseProviderSuite struct {
	lxctesting.TestSuite
	home    *coretesting.FakeHome
	restore func()
}

func (s *baseProviderSuite) SetUpTest(c *gc.C) {
	s.TestSuite.SetUpTest(c)
	s.home = coretesting.MakeFakeHomeNoEnvironments(c, "test")
	loggo.GetLogger("juju.provider.local").SetLogLevel(loggo.TRACE)
	s.restore = local.MockAddressForInterface()
}

func (s *baseProviderSuite) TearDownTest(c *gc.C) {
	s.restore()
	s.home.Restore()
	s.TestSuite.TearDownTest(c)
}

type prepareSuite struct {
	coretesting.FakeHomeSuite
}

var _ = gc.Suite(&prepareSuite{})

func (s *prepareSuite) SetUpTest(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	loggo.GetLogger("juju.provider.local").SetLogLevel(loggo.TRACE)
	s.PatchEnvironment("http_proxy", "")
	s.PatchEnvironment("HTTP_PROXY", "")
	s.PatchEnvironment("https_proxy", "")
	s.PatchEnvironment("HTTPS_PROXY", "")
	s.PatchEnvironment("ftp_proxy", "")
	s.PatchEnvironment("FTP_PROXY", "")
	s.PatchEnvironment("no_proxy", "")
	s.PatchEnvironment("NO_PROXY", "")
	s.HookCommandOutput(&utils.AptCommandOutput, nil, nil)
	s.PatchValue(local.CheckLocalPort, func(port int, desc string) error {
		return nil
	})
	restore := local.MockAddressForInterface()
	s.AddCleanup(func(*gc.C) { restore() })
}

func (s *prepareSuite) TestPrepareCapturesEnvironment(c *gc.C) {
	baseConfig, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": provider.Local,
		"name": "test",
	})
	c.Assert(err, gc.IsNil)
	provider, err := environs.Provider(provider.Local)
	c.Assert(err, gc.IsNil)

	for i, test := range []struct {
		message          string
		extraConfig      map[string]interface{}
		env              map[string]string
		aptOutput        string
		expectedProxy    osenv.ProxySettings
		expectedAptProxy osenv.ProxySettings
	}{{
		message: "nothing set",
	}, {
		message: "grabs proxy from environment",
		env: map[string]string{
			"http_proxy":  "http://user@10.0.0.1",
			"HTTPS_PROXY": "https://user@10.0.0.1",
			"ftp_proxy":   "ftp://user@10.0.0.1",
			"no_proxy":    "localhost,10.0.3.1",
		},
		expectedProxy: osenv.ProxySettings{
			Http:    "http://user@10.0.0.1",
			Https:   "https://user@10.0.0.1",
			Ftp:     "ftp://user@10.0.0.1",
			NoProxy: "localhost,10.0.3.1",
		},
		expectedAptProxy: osenv.ProxySettings{
			Http:  "http://user@10.0.0.1",
			Https: "https://user@10.0.0.1",
			Ftp:   "ftp://user@10.0.0.1",
		},
	}, {
		message: "skips proxy from environment if http-proxy set",
		extraConfig: map[string]interface{}{
			"http-proxy": "http://user@10.0.0.42",
		},
		env: map[string]string{
			"http_proxy":  "http://user@10.0.0.1",
			"HTTPS_PROXY": "https://user@10.0.0.1",
			"ftp_proxy":   "ftp://user@10.0.0.1",
		},
		expectedProxy: osenv.ProxySettings{
			Http: "http://user@10.0.0.42",
		},
		expectedAptProxy: osenv.ProxySettings{
			Http: "http://user@10.0.0.42",
		},
	}, {
		message: "skips proxy from environment if https-proxy set",
		extraConfig: map[string]interface{}{
			"https-proxy": "https://user@10.0.0.42",
		},
		env: map[string]string{
			"http_proxy":  "http://user@10.0.0.1",
			"HTTPS_PROXY": "https://user@10.0.0.1",
			"ftp_proxy":   "ftp://user@10.0.0.1",
		},
		expectedProxy: osenv.ProxySettings{
			Https: "https://user@10.0.0.42",
		},
		expectedAptProxy: osenv.ProxySettings{
			Https: "https://user@10.0.0.42",
		},
	}, {
		message: "skips proxy from environment if ftp-proxy set",
		extraConfig: map[string]interface{}{
			"ftp-proxy": "ftp://user@10.0.0.42",
		},
		env: map[string]string{
			"http_proxy":  "http://user@10.0.0.1",
			"HTTPS_PROXY": "https://user@10.0.0.1",
			"ftp_proxy":   "ftp://user@10.0.0.1",
		},
		expectedProxy: osenv.ProxySettings{
			Ftp: "ftp://user@10.0.0.42",
		},
		expectedAptProxy: osenv.ProxySettings{
			Ftp: "ftp://user@10.0.0.42",
		},
	}, {
		message: "skips proxy from environment if no-proxy set",
		extraConfig: map[string]interface{}{
			"no-proxy": "localhost,10.0.3.1",
		},
		env: map[string]string{
			"http_proxy":  "http://user@10.0.0.1",
			"HTTPS_PROXY": "https://user@10.0.0.1",
			"ftp_proxy":   "ftp://user@10.0.0.1",
		},
		expectedProxy: osenv.ProxySettings{
			NoProxy: "localhost,10.0.3.1",
		},
	}, {
		message: "apt-proxies detected",
		aptOutput: `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "false";
Acquire::ftp::Proxy "none";
Acquire::magic::Proxy "none";
`,
		expectedAptProxy: osenv.ProxySettings{
			Http:  "10.0.3.1:3142",
			Https: "false",
			Ftp:   "none",
		},
	}, {
		message: "apt-proxies not used if apt-http-proxy set",
		extraConfig: map[string]interface{}{
			"apt-http-proxy": "value-set",
		},
		aptOutput: `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "false";
Acquire::ftp::Proxy "none";
Acquire::magic::Proxy "none";
`,
		expectedAptProxy: osenv.ProxySettings{
			Http: "value-set",
		},
	}, {
		message: "apt-proxies not used if apt-https-proxy set",
		extraConfig: map[string]interface{}{
			"apt-https-proxy": "value-set",
		},
		aptOutput: `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "false";
Acquire::ftp::Proxy "none";
Acquire::magic::Proxy "none";
`,
		expectedAptProxy: osenv.ProxySettings{
			Https: "value-set",
		},
	}, {
		message: "apt-proxies not used if apt-ftp-proxy set",
		extraConfig: map[string]interface{}{
			"apt-ftp-proxy": "value-set",
		},
		aptOutput: `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "false";
Acquire::ftp::Proxy "none";
Acquire::magic::Proxy "none";
`,
		expectedAptProxy: osenv.ProxySettings{
			Ftp: "value-set",
		},
	}} {
		c.Logf("\n%v: %s", i, test.message)
		cleanup := []func(){}
		for key, value := range test.env {
			restore := testing.PatchEnvironment(key, value)
			cleanup = append(cleanup, restore)
		}
		_, restore := testing.HookCommandOutput(&utils.AptCommandOutput, []byte(test.aptOutput), nil)
		cleanup = append(cleanup, restore)
		testConfig := baseConfig
		if test.extraConfig != nil {
			testConfig, err = baseConfig.Apply(test.extraConfig)
			c.Assert(err, gc.IsNil)
		}
		env, err := provider.Prepare(coretesting.Context(c), testConfig)
		c.Assert(err, gc.IsNil)

		envConfig := env.Config()
		c.Assert(envConfig.HttpProxy(), gc.Equals, test.expectedProxy.Http)
		c.Assert(envConfig.HttpsProxy(), gc.Equals, test.expectedProxy.Https)
		c.Assert(envConfig.FtpProxy(), gc.Equals, test.expectedProxy.Ftp)
		c.Assert(envConfig.NoProxy(), gc.Equals, test.expectedProxy.NoProxy)

		c.Assert(envConfig.AptHttpProxy(), gc.Equals, test.expectedAptProxy.Http)
		c.Assert(envConfig.AptHttpsProxy(), gc.Equals, test.expectedAptProxy.Https)
		c.Assert(envConfig.AptFtpProxy(), gc.Equals, test.expectedAptProxy.Ftp)

		for _, clean := range cleanup {
			clean()
		}
	}
}

func (s *prepareSuite) TestPrepareNamespace(c *gc.C) {
	s.PatchValue(local.DetectAptProxies, func() (osenv.ProxySettings, error) {
		return osenv.ProxySettings{}, nil
	})
	basecfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": "local",
		"name": "test",
	})
	provider, err := environs.Provider("local")
	c.Assert(err, gc.IsNil)

	type test struct {
		userEnv   string
		userOS    string
		userOSErr error
		namespace string
		err       string
	}
	tests := []test{{
		userEnv:   "someone",
		userOS:    "other",
		namespace: "someone-test",
	}, {
		userOS:    "other",
		namespace: "other-test",
	}, {
		userOSErr: errors.New("oh noes"),
		err:       "failed to determine username for namespace: oh noes",
	}}

	for i, test := range tests {
		c.Logf("test %d: %v", i, test)
		s.PatchEnvironment("USER", test.userEnv)
		s.PatchValue(local.UserCurrent, func() (*user.User, error) {
			return &user.User{Username: test.userOS}, test.userOSErr
		})
		env, err := provider.Prepare(coretesting.Context(c), basecfg)
		if test.err == "" {
			c.Assert(err, gc.IsNil)
			cfg := env.Config()
			c.Assert(cfg.UnknownAttrs()["namespace"], gc.Equals, test.namespace)
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *prepareSuite) TestFastLXCClone(c *gc.C) {
	s.PatchValue(local.DetectAptProxies, func() (osenv.ProxySettings, error) {
		return osenv.ProxySettings{}, nil
	})
	s.PatchValue(&kvm.IsKVMSupported, func() (bool, error) {
		return true, nil
	})
	s.PatchValue(&local.VerifyPrerequisites, func(containerType instance.ContainerType) error {
		return nil
	})
	basecfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": "local",
		"name": "test",
	})
	provider, err := environs.Provider("local")
	c.Assert(err, gc.IsNil)

	type test struct {
		systemDefault bool
		extraConfig   map[string]interface{}
		expectClone   bool
		expectAUFS    bool
	}
	tests := []test{{
		extraConfig: map[string]interface{}{
			"container": "lxc",
		},
	}, {
		extraConfig: map[string]interface{}{
			"container": "lxc",
			"lxc-clone": "true",
		},
		expectClone: true,
	}, {
		systemDefault: true,
		extraConfig: map[string]interface{}{
			"container": "lxc",
		},
		expectClone: true,
	}, {
		systemDefault: true,
		extraConfig: map[string]interface{}{
			"container": "kvm",
		},
	}, {
		systemDefault: true,
		extraConfig: map[string]interface{}{
			"container": "lxc",
			"lxc-clone": false,
		},
	}, {
		systemDefault: true,
		extraConfig: map[string]interface{}{
			"container":      "lxc",
			"lxc-clone-aufs": true,
		},
		expectClone: true,
		expectAUFS:  true,
	}}

	for i, test := range tests {
		c.Logf("test %d: %v", i, test)

		releaseVersion := "12.04"
		if test.systemDefault {
			releaseVersion = "14.04"
		}
		s.PatchValue(local.ReleaseVersion, func() string { return releaseVersion })
		testConfig, err := basecfg.Apply(test.extraConfig)
		c.Assert(err, gc.IsNil)
		env, err := provider.Open(testConfig)
		c.Assert(err, gc.IsNil)
		localAttributes := env.Config().UnknownAttrs()

		value, _ := localAttributes["lxc-clone"].(bool)
		c.Assert(value, gc.Equals, test.expectClone)
		value, _ = localAttributes["lxc-clone-aufs"].(bool)
		c.Assert(value, gc.Equals, test.expectAUFS)
	}
}

func (s *prepareSuite) TestPrepareProxySSH(c *gc.C) {
	s.PatchValue(local.DetectAptProxies, func() (osenv.ProxySettings, error) {
		return osenv.ProxySettings{}, nil
	})
	basecfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": "local",
		"name": "test",
	})
	provider, err := environs.Provider("local")
	c.Assert(err, gc.IsNil)
	env, err := provider.Prepare(coretesting.Context(c), basecfg)
	c.Assert(err, gc.IsNil)
	// local provider sets proxy-ssh to false
	c.Assert(env.Config().ProxySSH(), gc.Equals, false)
}
