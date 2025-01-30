package main

import (
	"flag"
	"fmt"
	"log"

	"workloadstealagent/pkg/controller"
	informer "workloadstealagent/pkg/informer"
	"workloadstealagent/pkg/validate"

	"github.com/spf13/viper"
)

var (
	tlsKeyPath  string
	tlsCertPath string
)

func main() {
	stopChan := make(chan bool)

	validatorConfig := validate.Config{
		LableToFilter: getENVValue("WORK_LOAD_STEAL_LABLE"),
	}
	validator, err := validate.New(validatorConfig)
	if err != nil {
		log.Fatal(err)
	}

	controllerConfig := controller.Config{
		Port:        8443,
		TLSKeyPath:  tlsKeyPath,
		TLSCertPath: tlsCertPath,
	}
	server := controller.New(controllerConfig, validator)

	go log.Fatal(server.Start(stopChan))

	informerConfig := informer.Config{
		NATSURL:     getENVValue("NATS_URL"),
		NATSSubject: getENVValue("NATS_SUBJECT"),
	}
	informer, err := informer.New(informerConfig)
	if err != nil {
		log.Fatal(err)
	}

	go log.Fatal(informer.Start(stopChan))
}

func init() {
	flag.StringVar(&tlsKeyPath, "tlsKeyPath", "/etc/certs/tls.key", "Absolute path to the TLS key")
	flag.StringVar(&tlsCertPath, "tlsCertPath", "/etc/certs/tls.crt", "Absolute path to the TLS certificate")

	flag.Parse()
	// Initialize Viper
	viper.SetConfigType("env") // Use environment variables
	viper.AutomaticEnv()       // Automatically read environment variables
}

func getENVValue(envKey string) string {
	// Read environment variables
	value := viper.GetString("NATS_URL")
	if value == "" {
		message := fmt.Sprintf("%s environment variable is not set", envKey)
		log.Fatal(message)
	}
	return value
}
