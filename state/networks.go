// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
)

// networksDoc represents the network restrictions for a service or machine.
// The document ID field is the globalKey of a service or a machine.
type networksDoc struct {
	IncludeNetworks []string `bson:"include"`
	ExcludeNetworks []string `bson:"exclude"`
}

func newNetworksDoc(includeNetworks, excludeNetworks []string) *networksDoc {
	return &networksDoc{
		IncludeNetworks: includeNetworks,
		ExcludeNetworks: excludeNetworks,
	}
}

func createNetworksOp(st *State, id string, includeNetworks, excludeNetworks []string) txn.Op {
	return txn.Op{
		C:      st.networks.Name,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: newNetworksDoc(includeNetworks, excludeNetworks),
	}
}

// While networks are immutable, there is no setNetworksOp function.

func removeNetworksOp(st *State, id string) txn.Op {
	return txn.Op{
		C:      st.networks.Name,
		Id:     id,
		Remove: true,
	}
}

func readNetworks(st *State, id string) (includeNetworks, excludeNetworks []string, err error) {
	doc := networksDoc{}
	if err = st.networks.FindId(id).One(&doc); err == mgo.ErrNotFound {
		// In 1.17.7+ we always create a networksDoc for each service or
		// machine we create, but in legacy databases this is not the
		// case. We ignore the error here for backwards-compatibility.
		err = nil
	} else if err == nil {
		includeNetworks = doc.IncludeNetworks
		excludeNetworks = doc.ExcludeNetworks
	}
	return includeNetworks, excludeNetworks, err
}
