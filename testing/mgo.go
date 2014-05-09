// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/loggo"
	"labix.org/v2/mgo"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/utils"
)

var (
	// MgoServer is a shared mongo server used by tests.
	MgoServer = &MgoInstance{ssl: true}
	logger    = loggo.GetLogger("juju.testing")

	// regular expression to match output of mongod
	waitingForConnectionsRe = regexp.MustCompile(".*initandlisten.*waiting for connections.*")
)

type MgoInstance struct {
	// addr holds the address of the MongoDB server
	addr string

	// MgoPort holds the port of the MongoDB server.
	port int

	// server holds the running MongoDB command.
	server *exec.Cmd

	// exited receives a value when the mongodb server exits.
	exited <-chan struct{}

	// dir holds the directory that MongoDB is running in.
	dir string

	// ssl determines whether the MongoDB server will use TLS
	ssl bool

	// Params is a list of additional parameters that will be passed to
	// the mongod application
	Params []string
}

// Addr returns the address of the MongoDB server.
func (m *MgoInstance) Addr() string {
	return m.addr
}

// Port returns the port of the MongoDB server.
func (m *MgoInstance) Port() int {
	return m.port
}

// We specify a timeout to mgo.Dial, to prevent
// mongod failures hanging the tests.
const mgoDialTimeout = 15 * time.Second

// MgoSuite is a suite that deletes all content from the shared MongoDB
// server at the end of every test and supplies a connection to the shared
// MongoDB server.
type MgoSuite struct {
	Session *mgo.Session
}

// Start starts a MongoDB server in a temporary directory.
func (inst *MgoInstance) Start(ssl bool) error {
	dbdir, err := ioutil.TempDir("", "test-mgo")
	if err != nil {
		return err
	}
	logger.Debugf("starting mongo in %s", dbdir)

	// give them all the same keyfile so they can talk appropriately
	keyFilePath := filepath.Join(dbdir, "keyfile")
	err = ioutil.WriteFile(keyFilePath, []byte("not very secret"), 0600)
	if err != nil {
		return fmt.Errorf("cannot write key file: %v", err)
	}

	pemPath := filepath.Join(dbdir, "server.pem")
	err = ioutil.WriteFile(pemPath, []byte(ServerCert+ServerKey), 0600)
	if err != nil {
		return fmt.Errorf("cannot write cert/key PEM: %v", err)
	}
	inst.port = FindTCPPort()
	inst.addr = fmt.Sprintf("localhost:%d", inst.port)
	inst.dir = dbdir
	inst.ssl = ssl
	if err := inst.run(); err != nil {
		inst.addr = ""
		inst.port = 0
		os.RemoveAll(inst.dir)
		inst.dir = ""
		return err
	}
	logger.Debugf("started mongod pid %d in %s on port %d", inst.server.Process.Pid, dbdir, inst.port)
	return nil
}

// run runs the MongoDB server at the
// address and directory already configured.
func (inst *MgoInstance) run() error {
	if inst.server != nil {
		panic("mongo server is already running")
	}

	mgoport := strconv.Itoa(inst.port)
	mgoargs := []string{
		"--auth",
		"--dbpath", inst.dir,
		"--port", mgoport,
		"--nssize", "1",
		"--noprealloc",
		"--smallfiles",
		"--nojournal",
		"--nounixsocket",
		"--oplogSize", "10",
		"--keyFile", filepath.Join(inst.dir, "keyfile"),
	}
	if inst.ssl {
		mgoargs = append(mgoargs,
			"--sslOnNormalPorts",
			"--sslPEMKeyFile", filepath.Join(inst.dir, "server.pem"),
			"--sslPEMKeyPassword", "ignored")
	}
	if inst.Params != nil {
		mgoargs = append(mgoargs, inst.Params...)
	}
	server := exec.Command("mongod", mgoargs...)
	out, err := server.StdoutPipe()
	if err != nil {
		return err
	}
	server.Stderr = server.Stdout
	exited := make(chan struct{})
	started := make(chan struct{})
	listening := make(chan error, 1)
	go func() {
		<-started
		// Wait until the server is listening.
		var buf bytes.Buffer
		prefix := fmt.Sprintf("mongod:%v", mgoport)
		if readUntilMatching(prefix, io.TeeReader(out, &buf), waitingForConnectionsRe) {
			listening <- nil
		} else {
			listening <- fmt.Errorf("mongod failed to listen on port %v", mgoport)
		}
		// Capture the last 20 lines of output from mongod, to log
		// in the event of unclean exit.
		lines := readLastLines(prefix, io.MultiReader(&buf, out), 20)
		err = server.Wait()
		exitErr, _ := err.(*exec.ExitError)
		if err == nil || exitErr != nil && exitErr.Exited() {
			// mongodb has exited without being killed, so print the
			// last few lines of its log output.
			logger.Errorf("mongodb has exited without being killed")
			for _, line := range lines {
				logger.Errorf("mongod: %s", line)
			}
		}
		close(exited)
	}()
	inst.exited = exited
	err = server.Start()
	close(started)
	if err != nil {
		return err
	}
	err = <-listening
	close(listening)
	if err != nil {
		return err
	}
	inst.server = server

	return nil
}

func (inst *MgoInstance) kill() {
	inst.server.Process.Kill()
	<-inst.exited
	inst.server = nil
	inst.exited = nil
}

func (inst *MgoInstance) Destroy() {
	if inst.server != nil {
		logger.Debugf("killing mongod pid %d in %s on port %d", inst.server.Process.Pid, inst.dir, inst.port)
		inst.kill()
		os.RemoveAll(inst.dir)
		inst.addr, inst.dir = "", ""
	}
}

// Restart restarts the mongo server, useful for
// testing what happens when a state server goes down.
func (inst *MgoInstance) Restart() {
	logger.Debugf("restarting mongod pid %d in %s on port %d", inst.server.Process.Pid, inst.dir, inst.port)
	inst.kill()
	if err := inst.Start(inst.ssl); err != nil {
		panic(err)
	}
}

// MgoTestPackage should be called to register the tests for any package that
// requires a MongoDB server.
func MgoTestPackage(t *stdtesting.T) {
	MgoTestPackageSsl(t, true)
}

func MgoTestPackageSsl(t *stdtesting.T, ssl bool) {
	if err := MgoServer.Start(ssl); err != nil {
		t.Fatal(err)
	}
	defer MgoServer.Destroy()
	gc.TestingT(t)
}

func (s *MgoSuite) SetUpSuite(c *gc.C) {
	if MgoServer.addr == "" {
		panic("MgoSuite tests must be run with MgoTestPackage")
	}
	mgo.SetDebug(true)
	mgo.SetStats(true)
	// Make tests that use password authentication faster.
	utils.FastInsecureHash = true
}

// readUntilMatching reads lines from the given reader until the reader
// is depleted or a line matches the given regular expression.
func readUntilMatching(prefix string, r io.Reader, re *regexp.Regexp) bool {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		logger.Tracef("%s: %s", prefix, line)
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

// readLastLines reads lines from the given reader and returns
// the last n non-empty lines, ignoring empty lines.
func readLastLines(prefix string, r io.Reader, n int) []string {
	sc := bufio.NewScanner(r)
	lines := make([]string, n)
	i := 0
	for sc.Scan() {
		if line := strings.TrimRight(sc.Text(), "\n"); line != "" {
			logger.Tracef("%s: %s", prefix, line)
			lines[i%n] = line
			i++
		}
	}
	if err := sc.Err(); err != nil {
		panic(err)
	}
	final := make([]string, 0, n+1)
	if i > n {
		final = append(final, fmt.Sprintf("[%d lines omitted]", i-n))
	}
	for j := 0; j < n; j++ {
		if line := lines[(j+i)%n]; line != "" {
			final = append(final, line)
		}
	}
	return final
}

func (s *MgoSuite) TearDownSuite(c *gc.C) {
	utils.FastInsecureHash = false
}

// MustDial returns a new connection to the MongoDB server, and panics on
// errors.
func (inst *MgoInstance) MustDial() *mgo.Session {
	s, err := mgo.DialWithInfo(inst.DialInfo())
	if err != nil {
		panic(err)
	}
	return s
}

// Dial returns a new connection to the MongoDB server.
func (inst *MgoInstance) Dial() (*mgo.Session, error) {
	return mgo.DialWithInfo(inst.DialInfo())
}

// DialInfo returns information suitable for dialling the
// receiving MongoDB instance.
func (inst *MgoInstance) DialInfo() *mgo.DialInfo {
	return MgoDialInfoTls(inst.ssl, inst.addr)
}

// DialDirect returns a new direct connection to the shared MongoDB server. This
// must be used if you're connecting to a replicaset that hasn't been initiated
// yet.
func (inst *MgoInstance) DialDirect() (*mgo.Session, error) {
	info := inst.DialInfo()
	info.Direct = true
	return mgo.DialWithInfo(info)
}

// MustDialDirect works like DialDirect, but panics on errors.
func (inst *MgoInstance) MustDialDirect() *mgo.Session {
	session, err := inst.DialDirect()
	if err != nil {
		panic(err)
	}
	return session
}

// MgoDialInfo returns a DialInfo suitable
// for dialling an MgoInstance at any of the
// given addresses.
func MgoDialInfo(addrs ...string) *mgo.DialInfo {
	return MgoDialInfoTls(true, addrs...)
}

// MgoDialInfoTls returns a DialInfo suitable
// for dialling an MgoInstance at any of the
// given addresses, optionally using TLS.
func MgoDialInfoTls(useTls bool, addrs ...string) *mgo.DialInfo {
	var dial func(addr net.Addr) (net.Conn, error)
	if useTls {
		pool := x509.NewCertPool()
		xcert, err := cert.ParseCert(CACert)
		if err != nil {
			panic(err)
		}
		pool.AddCert(xcert)
		tlsConfig := &tls.Config{
			RootCAs:    pool,
			ServerName: "anything",
		}
		dial = func(addr net.Addr) (net.Conn, error) {
			conn, err := tls.Dial("tcp", addr.String(), tlsConfig)
			if err != nil {
				logger.Debugf("tls.Dial(%s) failed with %v", addr, err)
				return nil, err
			}
			return conn, nil
		}
	} else {
		dial = func(addr net.Addr) (net.Conn, error) {
			conn, err := net.Dial("tcp", addr.String())
			if err != nil {
				logger.Debugf("net.Dial(%s) failed with %v", addr, err)
				return nil, err
			}
			return conn, nil
		}
	}
	return &mgo.DialInfo{Addrs: addrs, Dial: dial, Timeout: mgoDialTimeout}
}

func (s *MgoSuite) SetUpTest(c *gc.C) {
	mgo.ResetStats()
	s.Session = MgoServer.MustDial()
}

// Reset deletes all content from the MongoDB server and panics if it encounters
// errors.
func (inst *MgoInstance) Reset() {
	// If the server has already been destroyed for testing purposes,
	// just start it again.
	if inst.Addr() == "" {
		if err := inst.Start(inst.ssl); err != nil {
			panic(err)
		}
		return
	}
	session := inst.MustDial()
	defer session.Close()

	dbnames, ok := resetAdminPasswordAndFetchDBNames(session)
	if !ok {
		// We restart it to regain access.  This should only
		// happen when tests fail.
		logger.Infof("restarting MongoDB server after unauthorized access")
		inst.Destroy()
		if err := inst.Start(inst.ssl); err != nil {
			panic(err)
		}
		return
	}
	logger.Infof("reset successfully reset admin password")
	for _, name := range dbnames {
		switch name {
		case "local", "config":
			// don't delete these
			continue
		}
		if err := session.DB(name).DropDatabase(); err != nil {
			panic(fmt.Errorf("Cannot drop MongoDB database %v: %v", name, err))
		}
	}
}

// resetAdminPasswordAndFetchDBNames logs into the database with a
// plausible password and returns all the database's db names. We need
// to try several passwords because we don't know what state the mongo
// server is in when Reset is called. If the test has set a custom
// password, we're out of luck, but if they are using
// DefaultStatePassword, we can succeed.
func resetAdminPasswordAndFetchDBNames(session *mgo.Session) ([]string, bool) {
	// First try with no password
	dbnames, err := session.DatabaseNames()
	if err == nil {
		return dbnames, true
	}
	if !isUnauthorized(err) {
		panic(err)
	}
	// Then try the two most likely passwords in turn.
	for _, password := range []string{
		DefaultMongoPassword,
		utils.UserPasswordHash(DefaultMongoPassword, utils.CompatSalt),
	} {
		admin := session.DB("admin")
		if err := admin.Login("admin", password); err != nil {
			logger.Infof("failed to log in with password %q", password)
			continue
		}
		dbnames, err := session.DatabaseNames()
		if err == nil {
			if err := admin.RemoveUser("admin"); err != nil {
				panic(err)
			}
			return dbnames, true
		}
		if !isUnauthorized(err) {
			panic(err)
		}
		logger.Infof("unauthorized access when getting database names; password %q", password)
	}
	return nil, false
}

// isUnauthorized is a copy of the same function in state/open.go.
func isUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	// Some unauthorized access errors have no error code,
	// just a simple error string.
	if err.Error() == "auth fails" {
		return true
	}
	if err, ok := err.(*mgo.QueryError); ok {
		return err.Code == 10057 ||
			err.Message == "need to login" ||
			err.Message == "unauthorized"
	}
	return false
}

func (s *MgoSuite) TearDownTest(c *gc.C) {
	MgoServer.Reset()
	s.Session.Close()
	for i := 0; ; i++ {
		stats := mgo.GetStats()
		if stats.SocketsInUse == 0 && stats.SocketsAlive == 0 {
			break
		}
		if i == 20 {
			c.Fatal("Test left sockets in a dirty state")
		}
		c.Logf("Waiting for sockets to die: %d in use, %d alive", stats.SocketsInUse, stats.SocketsAlive)
		time.Sleep(500 * time.Millisecond)
	}
}

// FindTCPPort finds an unused TCP port and returns it.
// Use of this function has an inherent race condition - another
// process may claim the port before we try to use it.
// We hope that the probability is small enough during
// testing to be negligible.
func FindTCPPort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
