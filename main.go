package main

import (
	"flag"
	"fmt"
	"github.com/aambhaik/tmgcagent/conf"
	"github.com/aambhaik/tmgcagent/consul"
	"github.com/julienschmidt/httprouter"
	"github.com/robfig/cron"
	_ "gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"gopkg.in/yaml.v2"
	"encoding/json"
)

var (
	configLocation    = flag.String("config", "/etc/tmgc/config.yaml", "location of the TMGC service configuration")
	validServiceTypes = []string{"Watch", "Timer", "Weather"}

	runBinary      = "binary"
	runScript      = "script"
	validExecTypes = []string{runBinary, runScript}

	shutdownManagedServiceImpact  = "shutdown-managed-service"
	suspendManagedServiceImpact   = "suspend-managed-service"
	reviveDependencyServiceImpact = "revive-dependency-service"
	validImpactTypes              = []string{shutdownManagedServiceImpact, suspendManagedServiceImpact, reviveDependencyServiceImpact}
	managedService                = conf.ManagedServiceInstance{}
)

var client *consul.ConsulClient
var runInterval string

func main() {
	//resolve any runtime flags
	flag.Parse()

	//read config that describes the managed process and its dependencies (format as per the TMGCAgentConfig definition)
	tmgcServiceConfig, err := getTMGCAgentConfiguration()
	if err != nil {
		log.Fatalf("Error with the service configuration: %v", err)
	}

	//get the run interval for the dependency check cron job
	runInterval = tmgcServiceConfig.ServiceAgent.DependencyCheckInterval

	//check managed service type.
	managedServiceConf := tmgcServiceConfig.ServiceAgent.ManagedService
	if !validateValue(managedServiceConf.Type, validServiceTypes) {
		log.Fatalf("invalid type found in the service configuration: %v, valid types are: %v", managedServiceConf.Type, validServiceTypes)
	}

	client, err := consul.NewConsulClient(tmgcServiceConfig.ServiceAgent.ServiceDiscovery.URL)
	if err != nil {
		log.Fatalf("Unable to connect to %v server running on : %v", tmgcServiceConfig.ServiceAgent.ServiceDiscovery.Type, tmgcServiceConfig.ServiceAgent.ServiceDiscovery.URL)
	}

	//contact consul service registry and get the callable URLs for the dependency service(s) described in the configuration.
	serviceDependencyURLsMap, err := discoverManagedServiceDependencies(client, tmgcServiceConfig)
	if err != nil {
		log.Fatalf("unable to resolve service dependency : %v", err)
	}

	//initiate the managed service
	managedProcess := managedServiceConf.Process.Exec
	managedProcessType := managedServiceConf.Process.Type
	if !validateValue(managedProcessType, validExecTypes) {
		log.Fatalf("invalid type found in the service configuration: %v, valid types are: %v", managedProcessType, validExecTypes)
	}

	var processArguments []string
	managedProcessArgNames := managedServiceConf.Process.Args
	for _, managedProcessArgName := range managedProcessArgNames {
		dependencyServiceURLs := serviceDependencyURLsMap[managedProcessArgName]
		for _, serviceURL := range dependencyServiceURLs {
			processArguments = append(processArguments, "-"+managedProcessArgName)
			processArguments = append(processArguments, serviceURL)
		}
	}
	command := exec.Command(managedProcess, processArguments...)

	//start the managed process
	startProcess(command)

	//announce the managed service to consul registry along with the ping check configuration.
	//right now, the ping check is based on HTTP Api check, but it can also be a TTL based health-check.
	//1. register the managed service
	serviceId, err := client.Register(nil, managedServiceConf.Name, "localhost", 9985, managedServiceConf.Type)
	if err != nil {
		log.Fatalf("Unable to register the service in Consul server running on : 127.0.0.1:8500")
	} else {
		//2. create the metadata for the managed service in consul.
		bytes, err := json.Marshal(tmgcServiceConfig)
		err = client.AddMetadata(*serviceId, bytes)
		if err != nil {
			log.Fatalf("Unable to add metadata to the service %v in Consul server running on : 127.0.0.1:8500, %v", serviceId, err)
		}
		log.Printf("service %v registered successfully", managedServiceConf.Name)
	}

	//create an in-memory struct to hold all the metadata about the managed process. this is necessary to support life-cycle operations, without using system-level calls.
	managedService = conf.ManagedServiceInstance{
		Command:   command,
		Name:      managedServiceConf.Name,
		Type:      managedServiceConf.Type,
		Config:    tmgcServiceConfig,
		Exec:      managedProcess,
		ServiceId: *serviceId,
	}

	//start a cron job that checks with consul if all the dependency services on which the managed service depends are healthy. the job, at present, also takes remediation action
	//on the managed service if the dependency services go bad. the remediation actions can be policy driven instead of arbitrary.
	checkDependencyHealthJob(client, tmgcServiceConfig)

	//start the service agent's own http routes to enable life-cycle management of the managed service.
	httpRoute(tmgcServiceConfig.ServiceAgent.ManagementPort)

	log.Printf("Agent for service %v started successfully", command.Path)
}

// read the configuration yaml
func getTMGCAgentConfiguration() (*conf.TMGCAgentConfig, error) {
	var sc *conf.TMGCAgentConfig

	jsonFile, err := ioutil.ReadFile(*configLocation)
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
		return nil, err
	}
	err = json.Unmarshal(jsonFile, &sc)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
		return nil, err
	}

	return sc, nil
}

// get addressable urls for the dependency services
func discoverManagedServiceDependencies(client *consul.ConsulClient, config *conf.TMGCAgentConfig) (serviceDepMap map[string][]string, err error) {
	dependencyURLsMap := make(map[string][]string)
	for _, service := range config.ServiceAgent.ManagedService.ServiceDependency {
		if service.Skip {
			continue
		}

		if !validateValue(service.ServiceType, validServiceTypes) {
			log.Printf("Invalid type found in the service configuration: %v, valid types are: %v", service.ServiceType, validServiceTypes)
			return nil, fmt.Errorf("invalid type found in the service configuration: %v", service.ServiceType)
		}

		if !validateValue(service.UnavailablityImpact, validImpactTypes) {
			log.Printf("Invalid impact type found in the service configuration: %v, valid types are: %v", service.UnavailablityImpact, validImpactTypes)
			return nil, fmt.Errorf("invalid type found in the service configuration: %v", service.UnavailablityImpact)
		}
		//query consul for service with specific Type
		dependencyServices, _, err := client.Service(service.ServiceName, service.ServiceType)
		if err != nil {
			log.Fatalf("Unable to access service from the registry. name: %v, type: %v", service.ServiceName, service.ServiceType)
			return nil, err
		}

		var urlList []string
		for _, dependencyService := range dependencyServices {
			dsHost := dependencyService.Service.Address
			if dsHost == "" {
				dsHost = "localhost"
			}
			dsPort := dependencyService.Service.Port

			tags := dependencyService.Service.Tags
			var relativePath string
			var protocol string
			for _, tag := range tags {
				if strings.HasPrefix(tag, "route:") {
					relativePath = tag[len("route:"):]
				} else if strings.HasPrefix(tag, "proto:") {
					protocol = tag[len("proto:"):]
				}
			}
			urlList = append(urlList, protocol+"://"+dsHost+":"+strconv.Itoa(dsPort)+relativePath)
		}
		dependencyURLsMap[service.EndpointMapping] = urlList
	}

	return dependencyURLsMap, nil
}

//cron job to check dependent service health
func checkDependencyHealthJob(client *consul.ConsulClient, config *conf.TMGCAgentConfig) {
	c := cron.New()

	c.AddFunc("@every "+runInterval, func() {
		err := managedService.Command.Process.Signal(syscall.Signal(0))

		if err != nil {
			log.Printf("Managed service %v is not running, skipping dependency check", managedService.Name)
		} else {
			for _, service := range config.ServiceAgent.ManagedService.ServiceDependency {
				if service.Skip {
					continue
				}
				//query consul for service with specific Type
				log.Printf("Checking dependency at %v", time.Now().Format("Jan 02 15:04:05.000 MST"))
				services, _, err := client.Service(service.ServiceName, service.ServiceType)
				if err != nil || services == nil {
					if service.UnavailablityImpact == shutdownManagedServiceImpact {
						log.Printf("Unable to access service from the registry. name: %v, type: %v", service.ServiceName, service.ServiceType)
						log.Printf("Shutting down the managed process %v ", managedService.Name)

						pgid, err := syscall.Getpgid(managedService.Command.Process.Pid)
						if err == nil {
							err := syscall.Kill(-pgid, syscall.SIGKILL)
							if err == nil {
								log.Printf("Managed service [%v] of type [%v] stopped successfully", managedService.Name, managedService.Type)
							} else {
								log.Printf("Error stopping the managed service [%v] of type [%v] : [%v]", managedService.Name, managedService.Type, err)
							}
						}

					} else if service.UnavailablityImpact == suspendManagedServiceImpact {
						log.Printf("Need to suspend the manged service. name: %v, type: %v", config.ServiceAgent.ManagedService.Name, config.ServiceAgent.ManagedService.Type)
					} else if service.UnavailablityImpact == reviveDependencyServiceImpact {
						log.Printf("Need to revive the managed service. name: %v, type: %v", service.ServiceName, service.ServiceType)
					}
				}
			}
		}
	})

	c.Start()
}

func startProcess(cmd *exec.Cmd) (bool, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		log.Printf("error starting the executable specified in the service configuration: %v", cmd.Path)
		return false, err
	}

	//avoid zombies??
	go cmd.Wait()

	return true, nil
}

func validateValue(value string, list []string) bool {
	for _, a := range list {
		if a == value {
			return true
		}
	}
	return false
}

/********************************************************************************************
	            life-cycle management of the managed service
 *******************************************************************************************/

//HTTP routes exposed by the service agent

//service agent http routes
func httpRoute(port int) {
	router := httprouter.New()
	router.GET("/agent/health", agentHealthHandler)
	router.GET("/service/health", managedServiceHealthHandler)
	router.PUT("/service/start", managedServiceStartHandler)
	router.PUT("/service/stop", managedServiceStopHandler)

	fmt.Printf("Starting Service Agent service on port %v", port)
	http.ListenAndServe(fmt.Sprintf("localhost:%v", port), router)
}

func agentHealthHandler(writer http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	writer.WriteHeader(200)
	writer.Write([]byte(fmt.Sprintf("Service Agent for managed service [%v] running successfully", managedService.Name)))
}

func managedServiceHealthHandler(writer http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	err := managedService.Command.Process.Signal(syscall.Signal(0))

	if err == nil {
		writer.WriteHeader(200)
		writer.Write([]byte(fmt.Sprintf("Managed Service [%v] of type [%v] running successfully", managedService.Name, managedService.Type)))
	} else {
		writer.WriteHeader(503)
		writer.Write([]byte(fmt.Sprintf("Managed Service [%v] of type [%v] is not running", managedService.Name, managedService.Type)))
	}
}

func managedServiceStartHandler(writer http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	if managedService.Command.Process.Signal(syscall.Signal(0)) == nil {
		//looks like the process is still running. can not start it again
		log.Printf("Unable to start the service %v, the process is already running", managedService.Name)
		writer.WriteHeader(500)
		writer.Write([]byte(fmt.Sprintf("Error starting the managed service [%v] of type [%v]", managedService.Name, managedService.Type)))
		return
	}
	process := managedService.Command
	command := exec.Command(managedService.Exec, process.Args...)
	_, err := startProcess(command)

	if err != nil {
		writer.WriteHeader(500)
		writer.Write([]byte(fmt.Sprintf("Error starting the managed service [%v] of type [%v]", managedService.Name, managedService.Type)))
	} else {
		//re-register service
		log.Println(managedService.ServiceId)
		serviceId, err := client.Register(&managedService.ServiceId, managedService.Name, "localhost", 9985, managedService.Type)
		if err != nil {
			log.Printf("unable to register the managed service in Consul server running on : 127.0.0.1:8500")
			writer.WriteHeader(500)
			writer.Write([]byte(fmt.Sprintf("Error starting the managed service [%v] of type [%v]", managedService.Name, managedService.Type)))

		} else {
			//create the metadata for the service
			bytes, err := yaml.Marshal(managedService.Config)
			err = client.AddMetadata(*serviceId, bytes)
			if err != nil {
				log.Printf("unable to add metadata to the managed service [%v] in Consul server running on : 127.0.0.1:8500", serviceId)
				writer.WriteHeader(500)
				writer.Write([]byte(fmt.Sprintf("Error starting the managed service [%v] of type [%v]", managedService.Name, managedService.Type)))
			}
			log.Printf("service %v started successfully", command.Path)
		}
		writer.WriteHeader(200)
		writer.Write([]byte(fmt.Sprintf("Managed service [%v] of type [%v] started successfully", managedService.Name, managedService.Type)))
	}
}

func managedServiceStopHandler(writer http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	pgid, err := syscall.Getpgid(managedService.Command.Process.Pid)
	if err == nil {
		err := syscall.Kill(-pgid, syscall.SIGKILL)
		if err == nil {
			writer.WriteHeader(200)
			writer.Write([]byte(fmt.Sprintf("Managed service [%v] of type [%v] stopped successfully", managedService.Name, managedService.Type)))
		} else {
			writer.WriteHeader(500)
			writer.Write([]byte(fmt.Sprintf("Error stopping the managed service [%v] of type [%v] : [%v]", managedService.Name, managedService.Type, err)))
		}
	} else {
		writer.WriteHeader(500)
		writer.Write([]byte(fmt.Sprintf("Error stopping the managed service [%v] of type [%v] : [%v]", managedService.Name, managedService.Type, err)))
	}
}
