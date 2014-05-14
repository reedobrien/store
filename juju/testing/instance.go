// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/network"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
)

// FakeStateInfo holds information about no state - it will always
// give an error when connected to.  The machine id gives the machine id
// of the machine to be started.
func FakeStateInfo(machineId string) *state.Info {
	return &state.Info{
		Addrs:    []string{"0.1.2.3:1234"},
		Tag:      names.MachineTag(machineId),
		Password: "unimportant",
		CACert:   testing.CACert,
	}
}

// FakeAPIInfo holds information about no state - it will always
// give an error when connected to.  The machine id gives the machine id
// of the machine to be started.
func FakeAPIInfo(machineId string) *api.Info {
	return &api.Info{
		Addrs:    []string{"0.1.2.3:1234"},
		Tag:      names.MachineTag(machineId),
		Password: "unimportant",
		CACert:   testing.CACert,
	}
}

// AssertStartInstance is a test helper function that starts an instance with a
// plausible but invalid configuration, and checks that it succeeds.
func AssertStartInstance(
	c *gc.C, env environs.Environ, machineId string,
) (
	instance.Instance, *instance.HardwareCharacteristics,
) {
	inst, hc, _, err := StartInstance(env, machineId)
	c.Assert(err, gc.IsNil)
	return inst, hc
}

// StartInstance is a test helper function that starts an instance with a plausible
// but invalid configuration, and returns the result of Environ.StartInstance.
func StartInstance(
	env environs.Environ, machineId string,
) (
	instance.Instance, *instance.HardwareCharacteristics, []network.Info, error,
) {
	return StartInstanceWithConstraints(env, machineId, constraints.Value{})
}

// AssertStartInstanceWithConstraints is a test helper function that starts an instance
// with the given constraints, and a plausible but invalid configuration, and returns
// the result of Environ.StartInstance.
func AssertStartInstanceWithConstraints(
	c *gc.C, env environs.Environ, machineId string, cons constraints.Value,
) (
	instance.Instance, *instance.HardwareCharacteristics,
) {
	inst, hc, _, err := StartInstanceWithConstraints(env, machineId, cons)
	c.Assert(err, gc.IsNil)
	return inst, hc
}

// StartInstanceWithConstraints is a test helper function that starts an instance
// with the given constraints, and a plausible but invalid configuration, and returns
// the result of Environ.StartInstance.
func StartInstanceWithConstraints(
	env environs.Environ, machineId string, cons constraints.Value,
) (
	instance.Instance, *instance.HardwareCharacteristics, []network.Info, error,
) {
	return StartInstanceWithConstraintsAndNetworks(env, machineId, cons, nil, nil)
}

// AssertStartInstanceWithNetworks is a test helper function that starts an
// instance with the given networks, and a plausible but invalid
// configuration, and returns the result of Environ.StartInstance.
func AssertStartInstanceWithNetworks(
	c *gc.C, env environs.Environ, machineId string, cons constraints.Value,
	includeNetworks, excludeNetworks []string,
) (
	instance.Instance, *instance.HardwareCharacteristics,
) {
	inst, hc, _, err := StartInstanceWithConstraintsAndNetworks(
		env, machineId, cons, includeNetworks, excludeNetworks)
	c.Assert(err, gc.IsNil)
	return inst, hc
}

// StartInstanceWithConstraintsAndNetworks is a test helper function that
// starts an instance with the given networks, and a plausible but invalid
// configuration, and returns the result of Environ.StartInstance.
func StartInstanceWithConstraintsAndNetworks(
	env environs.Environ, machineId string, cons constraints.Value,
	includeNetworks, excludeNetworks []string,
) (
	instance.Instance, *instance.HardwareCharacteristics, []network.Info, error,
) {
	series := config.PreferredSeries(env.Config())
	agentVersion, ok := env.Config().AgentVersion()
	if !ok {
		return nil, nil, nil, fmt.Errorf("missing agent version in environment config")
	}
	possibleTools, err := tools.FindInstanceTools(env, agentVersion, series, cons.Arch)
	if err != nil {
		return nil, nil, nil, err
	}
	machineNonce := "fake_nonce"
	stateInfo := FakeStateInfo(machineId)
	apiInfo := FakeAPIInfo(machineId)
	machineConfig := environs.NewMachineConfig(
		machineId, machineNonce,
		includeNetworks, excludeNetworks,
		stateInfo, apiInfo)
	return env.StartInstance(environs.StartInstanceParams{
		Constraints:   cons,
		Tools:         possibleTools,
		MachineConfig: machineConfig,
	})
}
