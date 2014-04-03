package mongo

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"labix.org/v2/mgo"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/juju/loggo"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/replicaset"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils"
)

const (
	maxFiles = 65000
	maxProcs = 20000

	serviceName = "juju-db"
	
	// SharedSecretFile is the name of the Mongo shared secret file
	// located within the Juju data directory.
	SharedSecretFile = "shared-secret"
)

var (
	logger = loggo.GetLogger("juju.agent.mongo")

	// JujuMongodPath holds the default path to the juju-specific mongod.
	JujuMongodPath = "/usr/lib/juju/bin/mongod"

	// MongodbServerPath holds the default path to the generic mongod.
	MongodbServerPath = "/usr/bin/mongod"
)

// WithAddresses represents an entity that has a set of
// addresses. e.g. a state Machine object
type WithAddresses interface {
	Addresses() []instance.Address
}

// IsMaster returns a boolean that represents whether the given
// machine's peer address is the primary mongo host for the replicaset
func IsMaster(session *mgo.Session, obj WithAddresses) (bool, error) {
	addrs := obj.Addresses()

	masterHostPort, err := replicaset.MasterHostPort(session)
	if err != nil {
		return false, err
	}

	masterAddr, _, err := net.SplitHostPort(masterHostPort)
	if err != nil {
		return false, err
	}

	machinePeerAddr := SelectPeerAddress(addrs)
	return machinePeerAddr == masterAddr, nil
}

// SelectPeerAddress returns the address to use as the
// mongo replica set peer address by selecting it from the given addresses.
func SelectPeerAddress(addrs []instance.Address) string {
	return instance.SelectInternalAddress(addrs, false)
}

// SelectPeerHostPort returns the HostPort to use as the
// mongo replica set peer by selecting it from the given hostPorts.
func SelectPeerHostPort(hostPorts []instance.HostPort) string {
	return instance.SelectInternalHostPort(hostPorts, false)
}

// GenerateSharedSecret generates a pseudo-random shared secret (keyfile)
// for use with Mongo replica sets.
func GenerateSharedSecret() (string, error) {
	// "A key’s length must be between 6 and 1024 characters and may
	// only contain characters in the base64 set."
	//   -- http://docs.mongodb.org/manual/tutorial/generate-key-file/
	buf := make([]byte, base64.StdEncoding.DecodedLen(1024))
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("cannot read random secret: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// MongoPackageForSeries returns the name of the mongo package for the series
// of the machine that it is going to be running on.
func MongoPackageForSeries(series string) string {
	switch series {
	case "precise", "quantal", "raring", "saucy":
		return "mongodb-server"
	default:
		// trusty and onwards
		return "juju-mongodb"
	}
}

// MongodPathForSeries returns the path to the mongod executable for the
// series of the machine that it is going to be running on.
func MongodPathForSeries(series string) string {
	if series == "trusty" {
		return JujuMongodPath
	}
	return MongodbServerPath
}

// MongoPath returns the executable path to be used to run mongod on this
// machine. If the juju-bundled version of mongo exists, it will return that
// path, otherwise it will return the command to run mongod from the path.
func MongodPath() (string, error) {
	if _, err := os.Stat(JujuMongodPath); err == nil {
		return JujuMongodPath, nil
	}

	path, err := exec.LookPath("mongod")
	if err != nil {
		return "", err
	}
	return path, nil
}

// RemoveService removes the mongoDB upstart service from this machine.
func RemoveService(namespace string) error {
	return upstart.NewService(ServiceName(namespace)).StopAndRemove()
}

// EnsureMongoServer ensures that the correct mongo upstart script is installed
// and running.
//
// This method will remove old versions of the mongo upstart script as necessary
// before installing the new version.
//
// The namespace is a unique identifier to prevent multiple instances of mongo
// on this machine from colliding. This should be empty unless using
// the local provider.
func EnsureMongoServer(dir string, port int, namespace string) error {
	// TODO: get the series from somewhere, non trusty values return
	// the existing default path.
	mongodPath := MongodPathForSeries("some-series")
	service, err := MongoUpstartService(namespace, mongodPath, dir, port)
	if err != nil {
		return err
	}

	if err := makeJournalDirs(dir); err != nil {
		return err
	}

	return service.Install()
}

// ServiceName returns the name of the upstart service config for mongo using
// the given namespace.
func ServiceName(namespace string) string {
	if namespace != "" {
		return fmt.Sprintf("%s-%s", serviceName, namespace)
	}
	return serviceName
}

func makeJournalDirs(dir string) error {
	journalDir := path.Join(dir, "journal")

	if err := os.MkdirAll(journalDir, 0700); err != nil {
		logger.Errorf("failed to make mongo journal dir %s: %v", journalDir, err)
		return err
	}

	// manually create the prealloc files, since otherwise they get created as 100M files.
	zeroes := make([]byte, 64*1024) // should be enough for anyone
	for x := 0; x < 3; x++ {
		name := fmt.Sprintf("prealloc.%d", x)
		filename := filepath.Join(journalDir, name)
		f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
		if err != nil {
			return fmt.Errorf("failed to open mongo prealloc file %q: %v", filename, err)
		}
		defer f.Close()
		for total := 0; total < 1024*1024; {
			n, err := f.Write(zeroes)
			if err != nil {
				return fmt.Errorf("failed to write to mongo prealloc file %q: %v", filename, err)
			}
			total += n
		}
	}
	return nil
}

// MongoUpstartService returns the upstart config for the mongo state service.
//
// This method assumes there exist "server.pem" and "shared_secret" keyfiles in dataDir.
func MongoUpstartService(namespace, mongodExec, dataDir string, port int) (*upstart.Conf, error) {
	// NOTE: ensure that the right package is installed?
	name := ServiceName(namespace)

	sslKeyFile := path.Join(dataDir, "server.pem")
	// TODO(Nate): uncomment when we commit HA stuff
	//keyFile := path.Join(dataDir, SharedSecretFile)
	svc := upstart.NewService(name)

	dbDir := path.Join(dataDir, "db")

	conf := &upstart.Conf{
		Service: *svc,
		Desc:    "juju state database",
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxFiles, maxFiles),
			"nproc":  fmt.Sprintf("%d %d", maxProcs, maxProcs),
		},
		Cmd: mongodExec +
			" --auth" +
			" --dbpath=" + dbDir +
			" --sslOnNormalPorts" +
			" --sslPEMKeyFile " + utils.ShQuote(sslKeyFile) +
			" --sslPEMKeyPassword ignored" +
			" --bind_ip 0.0.0.0" +
			" --port " + fmt.Sprint(port) +
			" --noprealloc" +
			" --syslog" +
			" --smallfiles",
		// TODO(Nate): uncomment when we commit HA stuff
		// +
		//	" --replSet juju" +
		//	" --keyFile " + utils.ShQuote(keyFile),
	}
	return conf, nil
}
