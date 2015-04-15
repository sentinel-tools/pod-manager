package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"text/template"

	"github.com/kelseyhightower/envconfig"
)

type LaunchConfig struct {
	SentinelConfigFile string
	RedSkullAddress    string
	ValidateNodes      bool
	UseRedSkull        bool
	UseSentinelConfig  bool
}

var config LaunchConfig
var enc *json.Encoder

// flags
var (
	podname           string
	info              bool
	jsonout           bool
	failover          bool
	reset             bool
	validatesentinels bool
)

func init() {
	err := envconfig.Process("podmanager", &config)
	if err != nil {
		log.Fatal(err)
	}
	// If we specify a source of pod info, set that source as what we want to use.
	if config.RedSkullAddress > "" {
		config.UseRedSkull = true
	}
	if config.SentinelConfigFile > "" {
		config.UseSentinelConfig = true
	}

	// now, set defaults for the source selected
	if config.UseRedSkull {
		if config.RedSkullAddress == "" {
			if config.UseRedSkull {
				config.RedSkullAddress = "localhost:8001"
			}
		}
	}

	if config.UseSentinelConfig {
		if config.SentinelConfigFile == "" {
			config.SentinelConfigFile = "/etc/redis/sentinel.conf"
		}
	}
	if !(config.UseSentinelConfig || config.UseRedSkull) {
		config.SentinelConfigFile = "/etc/redis/sentinel.conf"
		config.UseSentinelConfig = true
	}

}

func checkError(err error) {
	if err != nil {
		log.Fatalf("Error: %s", err)
	}
}

func main() {
	flag.StringVar(&podname, "podname", "", "Name of the pod")
	flag.BoolVar(&info, "info", false, "display pod info screen")
	flag.BoolVar(&failover, "failover", false, "initiate failover on pod")
	flag.BoolVar(&reset, "reset", false, "reset pod")
	flag.BoolVar(&jsonout, "jsonout", false, "output info in JSON format")
	flag.BoolVar(&validatesentinels, "validatesentinels", false, "check live sentinels vs known")
	flag.Parse()
	if podname == "" {
		log.Print("Need a podname. Try using '-podname <podname>'")
		flag.PrintDefaults()
		return
	}

	pod, err := getPod(podname)
	if err != nil {
		log.Fatal(err)
	}
	if jsonout {
		enc = json.NewEncoder(os.Stdout)
	}

	if info {
		if jsonout {
			if err := enc.Encode(&pod); err != nil {
				log.Println(err)
			}
		} else {
			t := template.Must(template.New("podinfo").Parse(PodInfoTemplate))
			err := t.Execute(os.Stdout, pod)
			if err != nil {
				log.Println("executing template:", err)
			}
			fmt.Printf("cli string: redis-cli -h %s -p %s -a %s\n", pod.MasterIP, pod.MasterPort, pod.Authpass)
		}
		return
	}
	if failover {
		err := Failover(pod)
		checkError(err)
		log.Print("Failover inititated")
	}
	if reset {
		err := Reset(pod)
		checkError(err)
		log.Print("Reset inititated")
	}
	if validatesentinels {
		val, err := ValidateSentinels(pod)
		checkError(err)
		if val {
			log.Print("sentinels validated")
		} else {
			log.Print("constellation state invalid for the pod")
		}
	}
}
