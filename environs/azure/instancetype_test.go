// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"

	gc "launchpad.net/gocheck"
	"launchpad.net/gwacl"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/jujutest"
)

type InstanceTypeSuite struct{}

var _ = gc.Suite(&InstanceTypeSuite{})

func (*InstanceTypeSuite) TestNewPreferredTypesAcceptsNil(c *gc.C) {
	types := newPreferredTypes(nil)

	c.Check(types, gc.HasLen, 0)
	c.Check(types.Len(), gc.Equals, 0)
}

func (*InstanceTypeSuite) TestNewPreferredTypesRepresentsInput(c *gc.C) {
	availableTypes := []gwacl.RoleSize{{Name: "Humongous", Cost: 123}}

	types := newPreferredTypes(availableTypes)

	c.Assert(types, gc.HasLen, len(availableTypes))
	c.Check(types[0], gc.Equals, &availableTypes[0])
	c.Check(types.Len(), gc.Equals, len(availableTypes))
}

func (*InstanceTypeSuite) TestNewPreferredTypesSortsByCost(c *gc.C) {
	availableTypes := []gwacl.RoleSize{
		{Name: "Excessive", Cost: 12},
		{Name: "Ridiculous", Cost: 99},
		{Name: "Modest", Cost: 3},
	}

	types := newPreferredTypes(availableTypes)

	c.Assert(types, gc.HasLen, len(availableTypes))
	// We end up with machine types sorted by ascending cost.
	c.Check(types[0].Name, gc.Equals, "Modest")
	c.Check(types[1].Name, gc.Equals, "Excessive")
	c.Check(types[2].Name, gc.Equals, "Ridiculous")
}

func (*InstanceTypeSuite) TestLessComparesCost(c *gc.C) {
	types := preferredTypes{
		{Name: "Cheap", Cost: 1},
		{Name: "Posh", Cost: 200},
	}

	c.Check(types.Less(0, 1), gc.Equals, true)
	c.Check(types.Less(1, 0), gc.Equals, false)
}

func (*InstanceTypeSuite) TestSwapSwitchesEntries(c *gc.C) {
	types := preferredTypes{
		{Name: "First"},
		{Name: "Last"},
	}

	types.Swap(0, 1)

	c.Check(types[0].Name, gc.Equals, "Last")
	c.Check(types[1].Name, gc.Equals, "First")
}

func (*InstanceTypeSuite) TestSwapIsCommutative(c *gc.C) {
	types := preferredTypes{
		{Name: "First"},
		{Name: "Last"},
	}

	types.Swap(1, 0)

	c.Check(types[0].Name, gc.Equals, "Last")
	c.Check(types[1].Name, gc.Equals, "First")
}

func (*InstanceTypeSuite) TestSwapLeavesOtherEntriesIntact(c *gc.C) {
	types := preferredTypes{
		{Name: "A"},
		{Name: "B"},
		{Name: "C"},
		{Name: "D"},
	}

	types.Swap(1, 2)

	c.Check(types[0].Name, gc.Equals, "A")
	c.Check(types[1].Name, gc.Equals, "C")
	c.Check(types[2].Name, gc.Equals, "B")
	c.Check(types[3].Name, gc.Equals, "D")
}

func (*InstanceTypeSuite) TestSufficesAcceptsNilRequirement(c *gc.C) {
	types := preferredTypes{}
	c.Check(types.suffices(0, nil), gc.Equals, true)
}

func (*InstanceTypeSuite) TestSufficesAcceptsMetRequirement(c *gc.C) {
	types := preferredTypes{}
	var expectation uint64 = 100
	c.Check(types.suffices(expectation+1, &expectation), gc.Equals, true)
}

func (*InstanceTypeSuite) TestSufficesAcceptsExactRequirement(c *gc.C) {
	types := preferredTypes{}
	var expectation uint64 = 100
	c.Check(types.suffices(expectation+1, &expectation), gc.Equals, true)
}

func (*InstanceTypeSuite) TestSufficesRejectsUnmetRequirement(c *gc.C) {
	types := preferredTypes{}
	var expectation uint64 = 100
	c.Check(types.suffices(expectation-1, &expectation), gc.Equals, false)
}

func (*InstanceTypeSuite) TestSatisfiesComparesCPUCores(c *gc.C) {
	types := preferredTypes{}
	var desiredCores uint64 = 5
	constraint := constraints.Value{CpuCores: &desiredCores}

	// A machine with fewer cores than required does not satisfy...
	machine := gwacl.RoleSize{CpuCores: desiredCores - 1}
	c.Check(types.satisfies(&machine, constraint), gc.Equals, false)
	// ...Even if it would, given more cores.
	machine.CpuCores = desiredCores
	c.Check(types.satisfies(&machine, constraint), gc.Equals, true)
}

func (*InstanceTypeSuite) TestSatisfiesComparesMem(c *gc.C) {
	types := preferredTypes{}
	var desiredMem uint64 = 37
	constraint := constraints.Value{Mem: &desiredMem}

	// A machine with less memory than required does not satisfy...
	machine := gwacl.RoleSize{Mem: desiredMem - 1}
	c.Check(types.satisfies(&machine, constraint), gc.Equals, false)
	// ...Even if it would, given more memory.
	machine.Mem = desiredMem
	c.Check(types.satisfies(&machine, constraint), gc.Equals, true)
}

func (*InstanceTypeSuite) TestDefaultToBaselineSpecSetsMimimumMem(c *gc.C) {
	c.Check(
		*defaultToBaselineSpec(constraints.Value{}).Mem,
		gc.Equals,
		uint64(defaultMem))
}

func (*InstanceTypeSuite) TestDefaultToBaselineSpecLeavesOriginalIntact(c *gc.C) {
	original := constraints.Value{}
	defaultToBaselineSpec(original)
	c.Check(original.Mem, gc.IsNil)
}

func (*InstanceTypeSuite) TestDefaultToBaselineSpecLeavesLowerMemIntact(c *gc.C) {
	const low = 100 * gwacl.MB
	var value uint64 = low
	c.Check(
		defaultToBaselineSpec(constraints.Value{Mem: &value}).Mem,
		gc.Equals,
		&value)
	c.Check(value, gc.Equals, uint64(low))
}

func (*InstanceTypeSuite) TestDefaultToBaselineSpecLeavesHigherMemIntact(c *gc.C) {
	const high = 100 * gwacl.MB
	var value uint64 = high
	c.Check(
		defaultToBaselineSpec(constraints.Value{Mem: &value}).Mem,
		gc.Equals,
		&value)
	c.Check(value, gc.Equals, uint64(high))
}

func (*InstanceTypeSuite) TestSelectMachineTypeReturnsErrorIfNoMatch(c *gc.C) {
	var lots uint64 = 1000000000000
	_, err := selectMachineType(nil, constraints.Value{Mem: &lots})
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "no machine type matches constraints mem=100000*[MGT]")
}

func (*InstanceTypeSuite) TestSelectMachineTypeReturnsCheapestMatch(c *gc.C) {
	var desiredCores uint64 = 50
	availableTypes := []gwacl.RoleSize{
		// Cheap, but not up to our requirements.
		{Name: "Panda", CpuCores: desiredCores / 2, Cost: 10},
		// Exactly what we need, but not the cheapest match.
		{Name: "LFA", CpuCores: desiredCores, Cost: 200},
		// Much more power than we need, but actually cheaper.
		{Name: "Lambo", CpuCores: 2 * desiredCores, Cost: 100},
		// Way out of our league.
		{Name: "Veyron", CpuCores: 10 * desiredCores, Cost: 500},
	}

	choice, err := selectMachineType(availableTypes, constraints.Value{CpuCores: &desiredCores})
	c.Assert(err, gc.IsNil)

	// Out of these options, selectMachineType picks not the first; not
	// the cheapest; not the biggest; not the last; but the cheapest type
	// of machine that meets requirements.
	c.Check(choice.Name, gc.Equals, "Lambo")
}

// fakeSimpleStreamsScheme is a fake protocol which tests can use for their
// simplestreams base URLs.
const fakeSimpleStreamsScheme = "azure-simplestreams-test"

// testRoundTripper is a fake http-like transport for injecting fake
// simplestream responses into these tests.
var testRoundTripper = jujutest.ProxyRoundTripper{}

func init() {
	// Route any request for a URL on the fakeSimpleStreamsScheme protocol
	// to testRoundTripper.
	testRoundTripper.RegisterForScheme(fakeSimpleStreamsScheme)
}

// prepareSimpleStreamsResponse sets up a fake response for our query to
// SimpleStreams.
//
// It returns a cleanup function, which you must call to reset things when
// done.
func prepareSimpleStreamsResponse(location, series, release, arch, json string) func() {
	fakeURL := fakeSimpleStreamsScheme + "://"
	originalURLs := baseURLs
	baseURLs = []string{fakeURL}

	originalSignedOnly := signedImageDataOnly
	signedImageDataOnly = false

	// Generate an index.  It will point to an Azure index with the
	// caller's json.
	index := fmt.Sprintf(`
		{
		 "index": {
		  "com.ubuntu.cloud:released:%s": {
		   "updated": "Tue, 30 Jul 2013 10:24:31 +0000",
		   "clouds": [
			{
			 "region": %q,
			 "endpoint": "https://management.core.windows.net/"
			}
		   ],
		   "cloudname": "azure",
		   "datatype": "image-ids",
		   "format": "products:1.0",
		   "products": [
			"com.ubuntu.cloud:server:%s:%s"
		   ],
		   "path": "/v1/azure.json"
		  }
		 },
		 "updated": "Tue, 30 Jul 2013 10:24:31 +0000",
		 "format": "index:1.0"
		}
		`, series, location, release, arch)
	files := map[string]string{
		"/v1/index.json": index,
		"/v1/azure.json": json,
	}
	testRoundTripper.Sub = jujutest.NewCannedRoundTripper(files, nil)
	return func() {
		baseURLs = originalURLs
		signedImageDataOnly = originalSignedOnly
		testRoundTripper.Sub = nil
	}
}

func (*InstanceTypeSuite) TestFindMatchingImagesReturnsErrorIfNoneFound(c *gc.C) {
	emptyResponse := `
		{
		 "format": "products:1.0"
		}
		`
	cleanup := prepareSimpleStreamsResponse("West US", "precise", "12.04", "amd64", emptyResponse)
	defer cleanup()

	_, err := findMatchingImages("West US", "precise", []string{"amd64"})
	c.Assert(err, gc.NotNil)

	c.Check(err, gc.ErrorMatches, "no OS images found for location .*")
}

func (*InstanceTypeSuite) TestFindMatchingImagesReturnsImages(c *gc.C) {
	// Real-world simplestreams index, pared down to a minimum:
	response := `
	{
	 "updated": "Tue, 09 Jul 2013 22:35:10 +0000",
	 "datatype": "image-ids",
	 "content_id": "com.ubuntu.cloud:released:azure",
	 "products": {
	  "com.ubuntu.cloud:server:12.04:amd64": {
	   "release": "precise",
	   "version": "12.04",
	   "arch": "amd64",
	   "versions": {
	    "20130603": {
	     "items": {
	      "euww1i3": {
	       "virt": "Hyper-V",
	       "crsn": "West Europe",
	       "root_size": "30GB",
	       "id": "MATCHING-IMAGE"
	      }
	     },
	     "pub_name": "b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-12_04_2-LTS-amd64-server-20130603-en-us-30GB",
	     "pub_label": "Ubuntu Server 12.04.2 LTS",
	     "label": "release"
	    }
	   }
	  }
	 },
	 "format": "products:1.0",
	 "_aliases": {
	  "crsn": {
	   "West Europe": {
	    "region": "West Europe",
	    "endpoint": "https://management.core.windows.net/"
	   }
	  }
	 }
	}
	`
	cleanup := prepareSimpleStreamsResponse("West Europe", "precise", "12.04", "amd64", response)
	defer cleanup()

	images, err := findMatchingImages("West Europe", "precise", []string{"amd64"})
	c.Assert(err, gc.IsNil)

	c.Assert(images, gc.HasLen, 1)
	c.Check(images[0].Id, gc.Equals, "MATCHING-IMAGE")
}