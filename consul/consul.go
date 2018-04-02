package consul

import (
	"fmt"

	"bytes"
	"crypto/rand"
	"encoding/gob"
	consul "github.com/hashicorp/consul/api"
	"io"
	"log"
	"strconv"
	"time"
)

//Client provides an interface for getting data out of Consul
type Client interface {
	// Get a Service from consul
	Service(string, string) ([]string, error)
	// Register a service with local agent
	Register(string, int) error
	// Deregister a service with local agent
	DeRegister(string) error
}

type ConsulClient struct {
	consul *consul.Client
}

//NewConsul returns a Client interface for given consul address
func NewConsulClient(addr string) (*ConsulClient, error) {
	config := consul.DefaultConfig()
	config.Address = addr
	c, err := consul.NewClient(config)
	if err != nil {
		return nil, err
	}
	return &ConsulClient{consul: c}, nil
}

// Register a service with consul local agent
func (c *ConsulClient) Register(id *string, name string, host string, port int, serviceType string) (sid *string, err error) {
	var serviceId string
	if id == nil {
		uniqueId, err := newUUID()
		if err != nil {
			return nil, err
		}
		serviceId = name + "-" + serviceType + "-" + uniqueId
	} else {
		serviceId = *id
	}

	reg := &consul.AgentServiceRegistration{
		ID:      serviceId,
		Name:    name,
		Address: host,
		Port:    port,
		Tags:    []string{serviceType, time.Now().Format("Jan 02 15:04:05.000 MST")},
		Check: &consul.AgentServiceCheck{
			HTTP:     "http://" + host + ":" + strconv.Itoa(port) + "/ping",
			Interval: "10s",
			Timeout:  "1s",
			Notes:    "Basic ping checks",
		},
	}
	return &serviceId, c.consul.Agent().ServiceRegister(reg)
}

// DeRegister a service with consul local agent
func (c *ConsulClient) DeRegister(id string) error {
	return c.consul.Agent().ServiceDeregister(id)
}

// Service return a service
func (c *ConsulClient) Service(service, tag string) ([]*consul.ServiceEntry, *consul.QueryMeta, error) {
	passingOnly := true
	addrs, meta, err := c.consul.Health().Service(service, tag, passingOnly, nil)
	if len(addrs) == 0 && err == nil {
		log.Printf("service ( %s ) was not found", service)
		return nil, nil, fmt.Errorf("service ( %s ) was not found", service)
	}
	if err != nil {
		log.Printf("Unexpected error ( %v ) in accessing the service", err)
		return nil, nil, err
	}
	return addrs, meta, nil
}

// Attach key-value metadata to a service
func (c *ConsulClient) AddMetadata(key string, value []byte) error {
	d := consul.KVPair{Key: key, Value: value}
	_, err := c.consul.KV().Put(&d, nil)
	if err != nil {
		log.Printf("error saving key/valye in consul KVP %v", d)
		return err
	}
	return nil
}

/**
Utility functions
*/

// get bytes from an arbitrary interface struct
func getBytes(data interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(data)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// get interface back from the byte array
func getInterface(bts []byte, data interface{}) error {
	buf := bytes.NewBuffer(bts)
	dec := gob.NewDecoder(buf)
	err := dec.Decode(data)
	if err != nil {
		return err
	}
	return nil
}

// newUUID generates a random UUID according to RFC 4122
func newUUID() (string, error) {
	uuid := make([]byte, 16)
	n, err := io.ReadFull(rand.Reader, uuid)
	if n != len(uuid) || err != nil {
		return "", err
	}
	// variant bits; see section 4.1.1
	uuid[8] = uuid[8]&^0xc0 | 0x80
	// version 4 (pseudo-random); see section 4.1.3
	uuid[6] = uuid[6]&^0xf0 | 0x40
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}
