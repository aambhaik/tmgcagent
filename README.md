# TMGC integration with Consul service registry



## Installation
### Prerequisites
* Consul [consul](https://www.docker.com/products/docker-toolbox) 
* TMGC [TMGC](https://github.com/aambhaik/tmgcagent)

## Getting Started

### Get the source code

	git clone https://github.com/aambhaik/tmgcagent tmgcagent
	git clone https://github.com/aambhaik/rolex rolex
	git clone https://github.com/aambhaik/timeservice timeservice
	
### Compile the binaries
	$jdoe-machine:cd tmgcagent
	$jdoe-machine:go install ./...
	
	$jdoe-machine:cd rolex
	$jdoe-machine:go install ./...
	
	$jdoe-machine:cd timeservice
	$jdoe-machine:go install ./...
	
The binaries are created under $GOPATH/bin folder.

### Start Consul server and register the timeservice with it
	1. Install consul (https://www.consul.io/docs/install/index.html#precompiled-binaries) & ensure that consul installation in included in the system path.
	2. Create a new directory /etc/consul.d 
	3. Copy the ping.json & timer.json into /etc/consul.d
	4. Start consul agent server with following command:
	   
	   consul agent -ui -dev -enable-script-checks -config-dir=/etc/consul.d

The consul server will be running and consul UI up on http://localhost:8500/ui

#### Start timeservice
	$jdoe-machine:timeservice
	Starting Time service on port 9980

	Check the consul UI and ensure that "TimerService" is registered with consul and running healthy.
	
#### Start tmgcagent (for Rolex service)
	
The TMGC agent is responsible for managing a service such as Rolex.

Rolex service, in turn, depends on the timer service and expects at least 1 instance of timer to be running.

When the TMGC agent starts, it looks up a configuration that describes
	a. the service that the agent is supposed to manage, aka the "managed service"
	b. the dependencies that the "managed service" expects to use.
	
The agent then checks with Consul registry and "discovers" the running instances of dependency services
specified in its configuration. It then maps the appropriate URLs to the managed service and spawns the
managaed service process as a child. The managed service, having been configured with all dependency URLs, now runs in a different process.

The agent then registers the managed service with Consul, attaches any meta-data and then performs cron checks. The cron job keeps track of any changes in the dependency service's health and decides on remediation actions based on the policy that the dependency service dictates (via configuration of course).

The agent also exposes REST end-points for:

	a. Health of the agent itself
	b. Health of the managed service
	c. Lifecycle operations on the managed service.
