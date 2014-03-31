// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"net"
	"strconv"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api/agent"
	"launchpad.net/juju-core/state/api/charmrevisionupdater"
	"launchpad.net/juju-core/state/api/deployer"
	"launchpad.net/juju-core/state/api/environment"
	"launchpad.net/juju-core/state/api/firewaller"
	"launchpad.net/juju-core/state/api/keyupdater"
	"launchpad.net/juju-core/state/api/logger"
	"launchpad.net/juju-core/state/api/machiner"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/provisioner"
	"launchpad.net/juju-core/state/api/rsyslog"
	"launchpad.net/juju-core/state/api/uniter"
	"launchpad.net/juju-core/state/api/upgrader"
)

// Login authenticates as the entity with the given name and password.
// Subsequent requests on the state will act as that entity.  This
// method is usually called automatically by Open. The machine nonce
// should be empty unless logging in as a machine agent.
func (st *State) Login(tag, password, nonce string) error {
	var result params.LoginResult
	err := st.Call("Admin", "", "Login", &params.Creds{
		AuthTag:  tag,
		Password: password,
		Nonce:    nonce,
	}, &result)
	if err == nil {
		st.authTag = tag
		hostPorts, err := addAddress(result.Servers, st.addr)
		if err != nil {
			st.Close()
			return err
		}
		st.hostPorts = hostPorts
	}
	return err
}

// addAddress appends a new server derived from the given
// address to servers if the address is not already found
// there.
func addAddress(servers [][]instance.HostPort, addr string) ([][]instance.HostPort, error) {
	for _, server := range servers {
		for _, hostPort := range server {
			if hostPort.NetAddr() == addr {
				return servers, nil
			}
		}
	}
	host, portString, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return nil, err
	}
	hostPort := instance.HostPort{
		Address: instance.Address{
			Value:        host,
			Type:         instance.DeriveAddressType(host),
			NetworkScope: instance.NetworkUnknown,
		},
		Port: port,
	}
	return append(servers, []instance.HostPort{hostPort}), nil
}

// Client returns an object that can be used
// to access client-specific functionality.
func (st *State) Client() *Client {
	return &Client{st}
}

// Machiner returns a version of the state that provides functionality
// required by the machiner worker.
func (st *State) Machiner() *machiner.State {
	return machiner.NewState(st)
}

// Provisioner returns a version of the state that provides functionality
// required by the provisioner worker.
func (st *State) Provisioner() *provisioner.State {
	return provisioner.NewState(st)
}

// Uniter returns a version of the state that provides functionality
// required by the uniter worker.
func (st *State) Uniter() *uniter.State {
	return uniter.NewState(st, st.authTag)
}

// Firewaller returns a version of the state that provides functionality
// required by the firewaller worker.
func (st *State) Firewaller() *firewaller.State {
	return firewaller.NewState(st)
}

// Agent returns a version of the state that provides
// functionality required by the agent code.
func (st *State) Agent() *agent.State {
	return agent.NewState(st)
}

// Upgrader returns access to the Upgrader API
func (st *State) Upgrader() *upgrader.State {
	return upgrader.NewState(st)
}

// Deployer returns access to the Deployer API
func (st *State) Deployer() *deployer.State {
	return deployer.NewState(st)
}

// Environment returns access to the Environment API
func (st *State) Environment() *environment.Facade {
	return environment.NewFacade(st)
}

// Logger returns access to the Logger API
func (st *State) Logger() *logger.State {
	return logger.NewState(st)
}

// KeyUpdater returns access to the KeyUpdater API
func (st *State) KeyUpdater() *keyupdater.State {
	return keyupdater.NewState(st)
}

// CharmRevisionUpdater returns access to the CharmRevisionUpdater API
func (st *State) CharmRevisionUpdater() *charmrevisionupdater.State {
	return charmrevisionupdater.NewState(st)
}

// Rsyslog returns access to the Rsyslog API
func (st *State) Rsyslog() *rsyslog.State {
	return rsyslog.NewState(st)
}
