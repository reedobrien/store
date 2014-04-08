// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

type imageSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&imageSuite{})

func (s *imageSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	UseTestImageData(TestImagesData)
}

func (s *imageSuite) TearDownSuite(c *gc.C) {
	UseTestImageData(nil)
	s.LoggingSuite.TearDownTest(c)
}

type specSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&specSuite{})

func (s *specSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	UseTestImageData(TestImagesData)
	UseTestInstanceTypeData(TestInstanceTypeCosts)
	UseTestRegionData(TestRegions)
}

func (s *specSuite) TearDownSuite(c *gc.C) {
	UseTestInstanceTypeData(nil)
	UseTestImageData(nil)
	UseTestRegionData(nil)
	s.LoggingSuite.TearDownSuite(c)
}

var findInstanceSpecTests = []struct {
	series string
	arches []string
	cons   string
	itype  string
	image  string
}{
	{
		series: "precise",
		arches: both,
		itype:  "m1.small",
		image:  "ami-00000033",
	}, {
		series: "quantal",
		arches: both,
		itype:  "m1.small",
		image:  "ami-01000034",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-cores=4",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-cores=2 arch=i386",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: "precise",
		arches: both,
		cons:   "mem=10G",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "mem=",
		itype:  "m1.small",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-power=",
		itype:  "m1.small",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-power=800",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-power=500 arch=i386",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: "precise",
		arches: []string{"i386"},
		cons:   "cpu-power=400",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: "quantal",
		arches: both,
		cons:   "arch=amd64",
		itype:  "cc1.4xlarge",
		image:  "ami-01000035",
	}, {
		series: "quantal",
		arches: both,
		cons:   "instance-type=cc1.4xlarge",
		itype:  "cc1.4xlarge",
		image:  "ami-01000035",
	}, {
		series: "precise",
		arches: []string{"i386"},
		cons:   "instance-type=cc1.4xlarge cpu-power=400",
		itype:  "c1.medium",
		image:  "ami-00000034",
	},
}

func (s *specSuite) TestFindInstanceSpec(c *gc.C) {
	for i, t := range findInstanceSpecTests {
		c.Logf("test %d", i)
		stor := ebsStorage
		spec, err := findInstanceSpec(
			[]simplestreams.DataSource{
				simplestreams.NewURLDataSource("test", "test:", utils.VerifySSLHostnames)},
			"released",
			&instances.InstanceConstraint{
				Region:      "test",
				Series:      t.series,
				Arches:      t.arches,
				Constraints: constraints.MustParse(t.cons),
				Storage:     &stor,
			})
		c.Assert(err, gc.IsNil)
		c.Check(spec.InstanceType.Name, gc.Equals, t.itype)
		c.Check(spec.Image.Id, gc.Equals, t.image)
	}
}

var findInstanceSpecErrorTests = []struct {
	series string
	arches []string
	cons   string
	err    string
}{
	{
		series: "bad",
		arches: both,
		err:    `invalid series "bad"`,
	}, {
		series: "precise",
		arches: []string{"arm"},
		err:    `no "precise" images in test with arches \[arm\]`,
	}, {
		series: "raring",
		arches: both,
		cons:   "mem=4G",
		err:    `no "raring" images in test matching instance types \[m1.large m1.xlarge c1.xlarge cc1.4xlarge cc2.8xlarge\]`,
	}, {
		series: "raring",
		arches: both,
		cons:   "instance-type=invalid",
		err:    `invalid instance type "invalid"`,
	},
}

func (s *specSuite) TestFindInstanceSpecErrors(c *gc.C) {
	for i, t := range findInstanceSpecErrorTests {
		c.Logf("test %d", i)
		_, err := findInstanceSpec(
			[]simplestreams.DataSource{
				simplestreams.NewURLDataSource("test", "test:", utils.VerifySSLHostnames)},
			"released",
			&instances.InstanceConstraint{
				Region:      "test",
				Series:      t.series,
				Arches:      t.arches,
				Constraints: constraints.MustParse(t.cons),
			})
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func (*specSuite) TestFilterImagesAcceptsNil(c *gc.C) {
	c.Check(filterImages(nil), gc.HasLen, 0)
}

func (*specSuite) TestFilterImagesAcceptsImageWithEBSStorage(c *gc.C) {
	input := []*imagemetadata.ImageMetadata{{Id: "yay", Storage: "ebs"}}
	c.Check(filterImages(input), gc.DeepEquals, input)
}

func (*specSuite) TestFilterImagesRejectsImageWithoutEBSStorage(c *gc.C) {
	input := []*imagemetadata.ImageMetadata{{Id: "boo", Storage: "ftp"}}
	c.Check(filterImages(input), gc.HasLen, 0)
}

func (*specSuite) TestFilterImagesReturnsSelectively(c *gc.C) {
	good := imagemetadata.ImageMetadata{Id: "good", Storage: "ebs"}
	bad := imagemetadata.ImageMetadata{Id: "bad", Storage: "ftp"}
	input := []*imagemetadata.ImageMetadata{&good, &bad}
	expectation := []*imagemetadata.ImageMetadata{&good}
	c.Check(filterImages(input), gc.DeepEquals, expectation)
}

func (*specSuite) TestFilterImagesMaintainsOrdering(c *gc.C) {
	input := []*imagemetadata.ImageMetadata{
		{Id: "one", Storage: "ebs"},
		{Id: "two", Storage: "ebs"},
		{Id: "three", Storage: "ebs"},
	}
	c.Check(filterImages(input), gc.DeepEquals, input)
}

var imageMatchConstraintTests = []struct{ in, out string }{
	{"arch=amd64", "arch=amd64"},
	{"arch=amd64 instance-type=foo", "arch=amd64"},
	{"instance-type=foo", "instance-type=foo"},
	{"root-disk=1G instance-type=foo", "root-disk=1G instance-type=foo"},
	{"arch=amd64 root-disk=1G instance-type=foo", "arch=amd64 root-disk=1G"},
}

func (s *specSuite) TestImageMatchConstraint(c *gc.C) {
	for _, test := range imageMatchConstraintTests {
		inCons := constraints.MustParse(test.in)
		outCons := constraints.MustParse(test.out)
		c.Check(imageMatchConstraint(inCons), gc.DeepEquals, outCons)
	}
}
