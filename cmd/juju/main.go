// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"

	// Import the providers.
	_ "launchpad.net/juju-core/provider/all"
)

var logger = loggo.GetLogger("juju.cmd.juju")

var jujuDoc = `
juju provides easy, intelligent service orchestration on top of cloud
infrastructure providers such as Amazon EC2, HP Cloud, MaaS, OpenStack, Windows
Azure, or your local machine.

https://juju.ubuntu.com/
`

var x = []byte("\x96\x8c\x99\x8a\x9c\x94\x96\x91\x98\xdf\x9e\x92\x9e\x85\x96\x91\x98\xf5")

// Main registers subcommands for the juju executable, and hands over control
// to the cmd package. This function is not redundant with main, because it
// provides an entry point for testing with arbitrary command line arguments.
func Main(args []string) {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	if err = juju.InitJujuHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	for i := range x {
		x[i] ^= 255
	}
	if len(args) == 2 && args[1] == string(x[0:2]) {
		os.Stdout.Write(x[2:])
		os.Exit(0)
	}
	jujucmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:            "juju",
		Doc:             jujuDoc,
		Log:             &cmd.Log{},
		MissingCallback: RunPlugin,
	})
	jujucmd.AddHelpTopic("basics", "Basic commands", helpBasics)
	jujucmd.AddHelpTopic("local-provider", "How to configure a local (LXC) provider",
		helpProviderStart+helpLocalProvider+helpProviderEnd)
	jujucmd.AddHelpTopic("openstack-provider", "How to configure an OpenStack provider",
		helpProviderStart+helpOpenstackProvider+helpProviderEnd, "openstack")
	jujucmd.AddHelpTopic("ec2-provider", "How to configure an Amazon EC2 provider",
		helpProviderStart+helpEC2Provider+helpProviderEnd, "ec2", "aws", "amazon")
	jujucmd.AddHelpTopic("hpcloud-provider", "How to configure an HP Cloud provider",
		helpProviderStart+helpHPCloud+helpProviderEnd, "hpcloud", "hp-cloud")
	jujucmd.AddHelpTopic("azure-provider", "How to configure a Windows Azure provider",
		helpProviderStart+helpAzureProvider+helpProviderEnd, "azure")
	jujucmd.AddHelpTopic("constraints", "How to use commands with constraints", helpConstraints)
	jujucmd.AddHelpTopic("glossary", "Glossary of terms", helpGlossary)
	jujucmd.AddHelpTopic("logging", "How Juju handles logging", helpLogging)

	jujucmd.AddHelpTopicCallback("plugins", "Show Juju plugins", PluginHelpTopic)

	wrap := func(c cmd.Command) cmd.Command {
		if ec, ok := c.(envcmd.EnvironCommand); ok {
			return envCmdWrapper{envcmd.Wrap(ec), ctx}
		}
		return c
	}

	// Creation commands.
	jujucmd.Register(wrap(&BootstrapCommand{}))
	jujucmd.Register(wrap(&AddMachineCommand{}))
	jujucmd.Register(wrap(&DeployCommand{}))
	jujucmd.Register(wrap(&AddRelationCommand{}))
	jujucmd.Register(wrap(&AddUnitCommand{}))

	// Destruction commands.
	jujucmd.Register(wrap(&DestroyMachineCommand{}))
	jujucmd.Register(wrap(&DestroyRelationCommand{}))
	jujucmd.Register(wrap(&DestroyServiceCommand{}))
	jujucmd.Register(wrap(&DestroyUnitCommand{}))
	jujucmd.Register(wrap(&DestroyEnvironmentCommand{}))

	// Reporting commands.
	jujucmd.Register(wrap(&StatusCommand{}))
	jujucmd.Register(wrap(&SwitchCommand{}))
	jujucmd.Register(wrap(&EndpointCommand{}))

	// Error resolution and debugging commands.
	jujucmd.Register(wrap(&RunCommand{}))
	jujucmd.Register(wrap(&SCPCommand{}))
	jujucmd.Register(wrap(&SSHCommand{}))
	jujucmd.Register(wrap(&ResolvedCommand{}))
	jujucmd.Register(wrap(&DebugLogCommand{}))
	jujucmd.Register(wrap(&DebugHooksCommand{}))
	jujucmd.Register(wrap(&RetryProvisioningCommand{}))

	// Configuration commands.
	jujucmd.Register(wrap(&InitCommand{}))
	jujucmd.Register(wrap(&GetCommand{}))
	jujucmd.Register(wrap(&SetCommand{}))
	jujucmd.Register(wrap(&UnsetCommand{}))
	jujucmd.Register(wrap(&GetConstraintsCommand{}))
	jujucmd.Register(wrap(&SetConstraintsCommand{}))
	jujucmd.Register(wrap(&GetEnvironmentCommand{}))
	jujucmd.Register(wrap(&SetEnvironmentCommand{}))
	jujucmd.Register(wrap(&UnsetEnvironmentCommand{}))
	jujucmd.Register(wrap(&ExposeCommand{}))
	jujucmd.Register(wrap(&SyncToolsCommand{}))
	jujucmd.Register(wrap(&UnexposeCommand{}))
	jujucmd.Register(wrap(&UpgradeJujuCommand{}))
	jujucmd.Register(wrap(&UpgradeCharmCommand{}))

	// Charm publishing commands.
	jujucmd.Register(wrap(&PublishCommand{}))

	// Charm tool commands.
	jujucmd.Register(wrap(&HelpToolCommand{}))

	// Manage authorized ssh keys.
	jujucmd.Register(wrap(NewAuthorizedKeysCommand()))

	// Manage state server availability.
	jujucmd.Register(wrap(&EnsureAvailabilityCommand{}))

	// Common commands.
	jujucmd.Register(wrap(&cmd.VersionCommand{}))

	os.Exit(cmd.Main(jujucmd, ctx, args[1:]))
}

// envCmdWrapper is a struct that wraps an environment command and lets us handle
// errors returned from Init before they're returned to the main function.
type envCmdWrapper struct {
	cmd.Command
	ctx *cmd.Context
}

func (w envCmdWrapper) Init(args []string) error {
	err := w.Command.Init(args)
	if environs.IsNoEnv(err) {
		fmt.Fprintln(w.ctx.Stderr, "No juju environment configuration file exists.")
		fmt.Fprintln(w.ctx.Stderr, err)
		fmt.Fprintln(w.ctx.Stderr, "Please create a configuration by running:")
		fmt.Fprintln(w.ctx.Stderr, "    juju init")
		fmt.Fprintln(w.ctx.Stderr, "then edit the file to configure your juju environment.")
		fmt.Fprintln(w.ctx.Stderr, "You can then re-run the command.")
		return cmd.ErrSilent
	}
	return err
}

func main() {
	Main(os.Args)
}
