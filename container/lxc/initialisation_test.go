// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

type InitialiserSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&InitialiserSuite{})

func (s *InitialiserSuite) TestLTSSeriesPackages(c *gc.C) {
	cmdChan := s.HookCommandOutput(&utils.AptCommandOutput, []byte{}, nil)
	container := NewContainerInitialiser("precise")
	err := container.Initialise()
	c.Assert(err, gc.IsNil)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-get", "--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
		"install", "--target-release", "precise-updates/cloud-tools", "lxc", "cloud-image-utils",
	})
}

func (s *InitialiserSuite) TestNoSeriesPackages(c *gc.C) {
	cmdChan := s.HookCommandOutput(&utils.AptCommandOutput, []byte{}, nil)
	container := NewContainerInitialiser("")
	err := container.Initialise()
	c.Assert(err, gc.IsNil)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-get", "--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
		"install", "lxc", "cloud-image-utils",
	})
}
