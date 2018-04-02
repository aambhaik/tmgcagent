package conf

import "os/exec"

type TMGCAgentConfig1 struct {
	ServiceAgent struct {
		DependencyCheckInterval string `yaml:"dependency-check-interval"`
		ManagedService          struct {
			Description string `yaml:"description"`
			Name        string `yaml:"name"`
			Process     struct {
				Args []string `yaml:"args"`
				Exec string   `yaml:"exec"`
				Type string   `yaml:"type"`
			} `yaml:"process"`
			ServiceDependency []struct {
				EndpointMapping     string `yaml:"endpoint-mapping"`
				MinInstances        int    `yaml:"min-instances"`
				ServiceName         string `yaml:"service-name"`
				ServiceType         string `yaml:"service-type"`
				Skip                bool   `yaml:"skip,omitempty"`
				UnavailablityImpact string `yaml:"unavailablity-impact"`
			} `yaml:"service-dependency"`
			Type string `yaml:"type"`
		} `yaml:"managed-service"`
		ManagementPort   int `yaml:"management-port"`
		ServiceDiscovery []struct {
			Type string `yaml:"type,omitempty"`
			URL  string `yaml:"url,omitempty"`
		} `yaml:"service-discovery"`
	} `yaml:"service-agent"`
}

type TMGCAgentConfig struct {
	ServiceAgent struct {
		ManagementPort   int `json:"management-port"`
		ServiceDiscovery struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"service-discovery"`
		ManagedService struct {
			Description string `json:"description"`
			Name        string `json:"name"`
			Process     struct {
				Args []string `json:"args"`
				Exec string   `json:"exec"`
				Type string   `json:"type"`
			} `json:"process"`
			ServiceDependency []struct {
				EndpointMapping     string `json:"endpoint-mapping"`
				ServiceName         string `json:"service-name"`
				ServiceType         string `json:"service-type"`
				Skip                bool   `json:"skip,omitempty"`
				UnavailablityImpact string `json:"unavailablity-impact"`
				MinInstances        int    `json:"min-instances,omitempty"`
			} `json:"service-dependency"`
			Type string `json:"type"`
		} `json:"managed-service"`
		DependencyCheckInterval string `json:"dependency-check-interval"`
	} `json:"service-agent"`
}

//type TMGCAgentConfig struct {
//	ServiceAgent struct {
//		DependencyCheckInterval string `yaml:"dependency-check-interval"`
//		ManagedService          ManagedServiceConfig
//		ManagementPort   int `yaml:"management-port"`
//		ServiceDiscovery []struct {
//			Type string `yaml:"type,omitempty"`
//			URL  string `yaml:"url,omitempty"`
//		} `yaml:"service-discovery"`
//	} `yaml:"service-agent"`
//}

//type ManagedServiceConfig struct {
//	Description string `yaml:"description"`
//	Name        string `yaml:"name"`
//	Process     struct {
//		Args  []string `yaml:"args"`
//		Exec string   `yaml:"exec"`
//		Type string   `yaml:"type"`
//	} `yaml:"process"`
//	ServiceDependency []struct {
//		EndpointMapping     string `yaml:"endpoint-mapping"`
//		MinInstances        int    `yaml:"min-instances,omitempty"`
//		ServiceName         string `yaml:"service-name"`
//		ServiceType         string `yaml:"service-type"`
//		Skip                bool   `yaml:"skip,omitempty"`
//		UnavailablityImpact string `yaml:"unavailablity-impact"`
//	} `yaml:"service-dependency"`
//	Type string `yaml:"type"`
//}

type ManagedServiceInstance struct {
	Command   *exec.Cmd
	Name      string
	Type      string
	ServiceId string
	Exec      string
	Config    *TMGCAgentConfig
}
