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
	walk              bool
	archive           bool
	jsonout           bool
	failover          bool
	reset             bool
	removepod         bool
	validatesentinels bool
	authcheck         bool
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
	flag.BoolVar(&removepod, "removepod", false, "remove pod from ALL sentinels which know about it")
	flag.BoolVar(&jsonout, "jsonout", false, "output info in JSON format")
	flag.BoolVar(&validatesentinels, "validatesentinels", false, "check live sentinels vs known")
	flag.BoolVar(&authcheck, "authcheck", false, "test auth to the pod")
	flag.BoolVar(&walk, "walk", false, "walk the config looking for IP pollution")
	flag.BoolVar(&archive, "archive", false, "on a delete, archive the config befored deleting")
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
	if removepod {
		if archive {
			t := template.Must(template.New("podinfo").Parse(PodInfoTemplate))
			filename := fmt.Sprintf("archive-%s.txt", podname)
			arcfile, err := os.Create(filename)
			if err != nil {
				log.Fatalf("Unable to open %s for writing archive, bailing", filename)
			}
			err = t.Execute(arcfile, pod)
			if err != nil {
				log.Fatal("executing template:", err)
			}
			fmt.Printf("cli string: redis-cli -h %s -p %s -a %s\n", pod.MasterIP, pod.MasterPort, pod.Authpass)
		}
		_, err := Remove(pod)
		checkError(err)
		log.Printf("Pod %s was removed from all sentinels", pod.Name)
	}
	if authcheck {
		res, err := CheckAuth(pod)
		if err != nil {
			log.Print(err)
			for k, v := range res {
				log.Printf("[%s] %s: %t", podname, k, v)
			}
		} else {
			log.Print("Auth valid")
		}
	}
	if walk {
		walked := make(map[string]bool)
		c := TreeWalk(*pod)
		if len(c) > 0 {
			walked[pod.Name] = true
			for _, mp := range c {
				_, exists := walked[mp.Name]
				if exists {
					continue
				}
				//CheckAuth(&mp)
				d := TreeWalk(mp)
				walked[mp.Name] = true
				for _, w3 := range d {
					_, exists := walked[w3.Name]
					if exists {
						continue
					}
					//CheckAuth(&w3)
					_ = TreeWalk(w3)
					walked[w3.Name] = true
				}
			}
		}
	}
}
