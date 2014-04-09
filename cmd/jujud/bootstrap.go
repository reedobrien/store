// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"

	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/agent/mongo"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

type BootstrapCommand struct {
	cmd.CommandBase
	AgentConf
	EnvConfig   map[string]interface{}
	Constraints constraints.Value
	Hardware    instance.HardwareCharacteristics
	InstanceId  string
}

// Info returns a decription of the command.
func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bootstrap-state",
		Purpose: "initialize juju state",
	}
}

func (c *BootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.AgentConf.AddFlags(f)
	yamlBase64Var(f, &c.EnvConfig, "env-config", "", "initial environment configuration (yaml, base64 encoded)")
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "initial environment constraints (space-separated strings)")
	f.Var(&c.Hardware, "hardware", "hardware characteristics (space-separated strings)")
	f.StringVar(&c.InstanceId, "instance-id", "", "unique instance-id for bootstrap machine")
}

// Init initializes the command for running.
func (c *BootstrapCommand) Init(args []string) error {
	if len(c.EnvConfig) == 0 {
		return requiredError("env-config")
	}
	if c.InstanceId == "" {
		return requiredError("instance-id")
	}
	return c.AgentConf.CheckArgs(args)
}

// Run initializes state for an environment.
func (c *BootstrapCommand) Run(_ *cmd.Context) error {
	envCfg, err := config.New(config.NoDefaults, c.EnvConfig)
	if err != nil {
		return err
	}
	err = c.ReadConfig("machine-0")
	if err != nil {
		return err
	}
	agentConfig := c.CurrentConfig()

	// agent.Jobs is an optional field in the agent config, and was
	// introduced after 1.17.2. We default to allowing units on
	// machine-0 if missing.
	jobs := agentConfig.Jobs()
	if len(jobs) == 0 {
		jobs = []params.MachineJob{
			params.JobManageEnviron,
			params.JobHostUnits,
		}
	}

	// Get the bootstrap machine's addresses from the provider.
	env, err := environs.New(envCfg)
	if err != nil {
		return err
	}
	instanceId := instance.Id(c.InstanceId)
	instances, err := env.Instances([]instance.Id{instanceId})
	if err != nil {
		return err
	}
	addrs, err := instances[0].Addresses()
	if err != nil {
		return err
	}

	// Generate a shared secret for the Mongo replica set, and write it out.
	sharedSecret, err := mongo.GenerateSharedSecret()
	if err != nil {
		return err
	}
	sharedSecretFile := filepath.Join(c.dataDir, mongo.SharedSecretFile)
	if err := ioutil.WriteFile(sharedSecretFile, []byte(sharedSecret), 0600); err != nil {
		return err
	}

	namespace := agentConfig.Value(agent.Namespace)

	if err := c.startMongo(addrs, envCfg.StatePort(), namespace); err != nil {
		return err
	}

	// Initialise state, and store any agent config (e.g. password) changes.
	var st *state.State
	err = nil
	writeErr := c.ChangeConfig(func(agentConfig agent.ConfigSetter) {
		st, _, err = agent.InitializeState(
			agentConfig,
			envCfg,
			agent.BootstrapMachineConfig{
				Addresses:       addrs,
				Constraints:     c.Constraints,
				Jobs:            jobs,
				InstanceId:      instanceId,
				Characteristics: c.Hardware,
				SharedSecret:    sharedSecret,
			},
			state.DefaultDialOpts(),
			environs.NewStatePolicy(),
		)
	})
	if writeErr != nil {
		return fmt.Errorf("cannot write initial configuration: %v", err)
	}
	if err != nil {
		return err
	}
	st.Close()
	return nil
}

func (c *BootstrapCommand) startMongo(addrs []instance.Address, port int, namespace string) error {
	logger.Debugf("starting mongo")

	agentConfig := c.CurrentConfig()
	dialInfo, err := state.DialInfo(agentConfig.StateInfo(), state.DefaultDialOpts())
	if err != nil {
		return err
	}
	// Use localhost to dial the mongo server, because it's running in
	// auth mode and will refuse to perform any operations unless
	// we dial that address.
	dialInfo.Addrs = []string{
		net.JoinHostPort("127.0.0.1", fmt.Sprint(port)),
	}

	if err := ensureMongoServer(agentConfig.DataDir(), port, namespace); err != nil {
		return err
	}

	memberAddr := fmt.Sprintf("%s:%d", mongo.SelectPeerAddress(addrs), port)

	return maybeInitiateMongoServer(mongo.InitiateMongoParams{
		DialInfo:       dialInfo,
		MemberHostPort: memberAddr,
	})
}

// yamlBase64Value implements gnuflag.Value on a map[string]interface{}.
type yamlBase64Value map[string]interface{}

// Set decodes the base64 value into yaml then expands that into a map.
func (v *yamlBase64Value) Set(value string) error {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	return goyaml.Unmarshal(decoded, v)
}

func (v *yamlBase64Value) String() string {
	return fmt.Sprintf("%v", *v)
}

// yamlBase64Var sets up a gnuflag flag analogous to the FlagSet.*Var methods.
func yamlBase64Var(fs *gnuflag.FlagSet, target *map[string]interface{}, name string, value string, usage string) {
	fs.Var((*yamlBase64Value)(target), name, usage)
}
