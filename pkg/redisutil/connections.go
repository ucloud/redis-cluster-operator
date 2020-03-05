package redisutil

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/mediocregopher/radix.v2/redis"
	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

const (
	defaultClientTimeout = 2 * time.Second
	defaultClientName    = ""

	// ErrNotFound cannot find a node to connect to
	ErrNotFound = "unable to find a node to connect"
)

// IAdminConnections interface representing the map of admin connections to redis cluster nodes
type IAdminConnections interface {
	// Add connect to the given address and
	// register the client connection to the pool
	Add(addr string) error
	// Remove disconnect and remove the client connection from the map
	Remove(addr string)
	// Get returns a client connection for the given address,
	// connects if the connection is not in the map yet
	Get(addr string) (IClient, error)
	// GetRandom returns a client connection to a random node of the client map
	GetRandom() (IClient, error)
	// GetDifferentFrom returns a random client connection different from given address
	GetDifferentFrom(addr string) (IClient, error)
	// GetAll returns a map of all clients per address
	GetAll() map[string]IClient
	//GetSelected returns a map of clients based on the input addresses
	GetSelected(addrs []string) map[string]IClient
	// Reconnect force a reconnection on the given address
	// if the adress is not part of the map, act like Add
	Reconnect(addr string) error
	// AddAll connect to the given list of addresses and
	// register them in the map
	// fail silently
	AddAll(addrs []string)
	// ReplaceAll clear the map and re-populate it with new connections
	// fail silently
	ReplaceAll(addrs []string)
	// ValidateResp check the redis resp, eventually reconnect on connection error
	// in case of error, customize the error, log it and return it
	ValidateResp(resp *redis.Resp, addr, errMessage string) error
	// ValidatePipeResp wait for all answers in the pipe and validate the response
	// in case of network issue clear the pipe and return
	// in case of error return false
	ValidatePipeResp(c IClient, addr, errMessage string) bool
	// Reset close all connections and clear the connection map
	Reset()
	// GetAUTH return password and true if connection password is set, else return false.
	GetAUTH() (string, bool)
}

// AdminConnections connection map for redis cluster
// currently the admin connection is not threadSafe since it is only use in the Events thread.
type AdminConnections struct {
	clients           map[string]IClient
	connectionTimeout time.Duration
	commandsMapping   map[string]string
	clientName        string
	password          string
	log               logr.Logger
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

// NewAdminConnections returns and instance of AdminConnectionsInterface
func NewAdminConnections(addrs []string, options *AdminOptions, log logr.Logger) IAdminConnections {
	cnx := &AdminConnections{
		clients:           make(map[string]IClient),
		connectionTimeout: defaultClientTimeout,
		commandsMapping:   make(map[string]string),
		clientName:        defaultClientName,
		log:               log,
	}
	if options != nil {
		if options.ConnectionTimeout != 0 {
			cnx.connectionTimeout = options.ConnectionTimeout
		}
		if _, err := os.Stat(options.RenameCommandsFile); err == nil {
			cnx.commandsMapping = utils.BuildCommandReplaceMapping(options.RenameCommandsFile, cnx.log)
		}
		cnx.clientName = options.ClientName
		cnx.password = options.Password
	}
	cnx.AddAll(addrs)
	return cnx
}

// GetAUTH return password and true if connection password is set, else return false.
func (cnx *AdminConnections) GetAUTH() (string, bool) {
	if len(cnx.password) > 0 {
		return cnx.password, true
	}
	return "", false
}

// Reconnect force a reconnection on the given address
// is the adress is not part of the map, act like Add
func (cnx *AdminConnections) Reconnect(addr string) error {
	cnx.log.Info(fmt.Sprintf("reconnecting to %s", addr))
	cnx.Remove(addr)
	return cnx.Add(addr)
}

// AddAll connect to the given list of addresses and
// register them in the map
// fail silently
func (cnx *AdminConnections) AddAll(addrs []string) {
	for _, addr := range addrs {
		cnx.Add(addr)
	}
}

// ReplaceAll clear the pool and re-populate it with new connections
// fail silently
func (cnx *AdminConnections) ReplaceAll(addrs []string) {
	cnx.Reset()
	cnx.AddAll(addrs)
}

// Reset close all connections and clear the connection map
func (cnx *AdminConnections) Reset() {
	for _, c := range cnx.clients {
		c.Close()
	}
	cnx.clients = map[string]IClient{}
}

// GetAll returns a map of all clients per address
func (cnx *AdminConnections) GetAll() map[string]IClient {
	return cnx.clients
}

//GetSelected returns a map of clients based on the input addresses
func (cnx *AdminConnections) GetSelected(addrs []string) map[string]IClient {
	clientsSelected := make(map[string]IClient)
	for _, addr := range addrs {
		if client, ok := cnx.clients[addr]; ok {
			clientsSelected[addr] = client
		}
	}
	return clientsSelected
}

// Close used to close all possible resources instanciate by the Connections
func (cnx *AdminConnections) Close() {
	for _, c := range cnx.clients {
		c.Close()
	}
}

// Add connect to the given address and
// register the client connection to the map
func (cnx *AdminConnections) Add(addr string) error {
	_, err := cnx.Update(addr)
	return err
}

// Remove disconnect and remove the client connection from the map
func (cnx *AdminConnections) Remove(addr string) {
	if c, ok := cnx.clients[addr]; ok {
		c.Close()
		delete(cnx.clients, addr)
	}
}

// Update returns a client connection for the given adress,
// connects if the connection is not in the map yet
func (cnx *AdminConnections) Update(addr string) (IClient, error) {
	// if already exist close the current connection
	if c, ok := cnx.clients[addr]; ok {
		c.Close()
	}

	c, err := cnx.connect(addr)
	if err == nil && c != nil {
		cnx.clients[addr] = c
	} else {
		cnx.log.Info(fmt.Sprintf("cannot connect to %s ", addr))
	}
	return c, err
}

// Get returns a client connection for the given adress,
// connects if the connection is not in the map yet
func (cnx *AdminConnections) Get(addr string) (IClient, error) {
	if c, ok := cnx.clients[addr]; ok {
		return c, nil
	}
	c, err := cnx.connect(addr)
	if err == nil && c != nil {
		cnx.clients[addr] = c
	}
	return c, err
}

// GetRandom returns a client connection to a random node of the client map
func (cnx *AdminConnections) GetRandom() (IClient, error) {
	_, c, err := cnx.getRandomKeyClient()
	return c, err
}

// GetDifferentFrom returns random a client connection different from given address
func (cnx *AdminConnections) GetDifferentFrom(addr string) (IClient, error) {
	if len(cnx.clients) == 1 {
		for a, c := range cnx.clients {
			if a != addr {
				return c, nil
			}
			return nil, errors.New(ErrNotFound)
		}
	}

	for {
		a, c, err := cnx.getRandomKeyClient()
		if err != nil {
			return nil, err
		}
		if a != addr {
			return c, nil
		}
	}
}

// GetRandom returns a client connection to a random node of the client map
func (cnx *AdminConnections) getRandomKeyClient() (string, IClient, error) {
	nbClient := len(cnx.clients)
	if nbClient == 0 {
		return "", nil, errors.New(ErrNotFound)
	}
	randNumber := rand.Intn(nbClient)
	for k, c := range cnx.clients {
		if randNumber == 0 {
			return k, c, nil
		}
		randNumber--
	}

	return "", nil, errors.New(ErrNotFound)
}

// handleError handle a network error, reconnects if necessary, in that case, returns true
func (cnx *AdminConnections) handleError(addr string, err error) bool {
	if err == nil {
		return false
	} else if netError, ok := err.(net.Error); ok && netError.Timeout() {
		// timeout, reconnect
		cnx.Reconnect(addr)
		return true
	}
	switch err.(type) {
	case *net.OpError:
		// connection refused, reconnect
		cnx.Reconnect(addr)
		return true
	}
	return false
}

func (cnx *AdminConnections) connect(addr string) (IClient, error) {
	c, err := NewClient(addr, cnx.password, cnx.connectionTimeout, cnx.commandsMapping)
	if err != nil {
		return nil, err
	}
	if cnx.clientName != "" {
		resp := c.Cmd("CLIENT", "SETNAME", cnx.clientName)
		return c, cnx.ValidateResp(resp, addr, "Unable to run command CLIENT SETNAME")
	}

	return c, nil
}

// ValidateResp check the redis resp, eventually reconnect on connection error
// in case of error, customize the error, log it and return it
func (cnx *AdminConnections) ValidateResp(resp *redis.Resp, addr, errMessage string) error {
	if resp == nil {
		cnx.log.Error(fmt.Errorf("%s: unable to connect to node %s", errMessage, addr), "")
		return fmt.Errorf("%s: unable to connect to node %s", errMessage, addr)
	}
	if resp.Err != nil {
		cnx.handleError(addr, resp.Err)
		cnx.log.Error(resp.Err, fmt.Sprintf("%s: unexpected error on node %s", errMessage, addr))
		return fmt.Errorf("%s: unexpected error on node %s: %v", errMessage, addr, resp.Err)
	}
	return nil
}

// ValidatePipeResp wait for all answers in the pipe and validate the response
// in case of network issue clear the pipe and return
// in case of error, return false
func (cnx *AdminConnections) ValidatePipeResp(client IClient, addr, errMessage string) bool {
	ok := true
	for {
		resp := client.PipeResp()
		if resp == nil {
			cnx.log.Error(fmt.Errorf("%s: unable to connect to node %s", errMessage, addr), "")
			return false
		}
		if resp.Err != nil {
			if resp.Err == redis.ErrPipelineEmpty {
				break
			}
			cnx.log.Error(fmt.Errorf("%s: unexpected error on node %s: %v", errMessage, addr, resp.Err), "")
			if cnx.handleError(addr, resp.Err) {
				// network error, no need to continue
				return false
			}
			ok = false
		}
	}

	return ok
}
