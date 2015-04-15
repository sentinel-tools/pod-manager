// +build !redskull
package main

import (
	"fmt"
	"log"

	"github.com/sentinel-tools/sconf-parser"
	"github.com/therealbill/libredis/client"
)

//getPod(podname) returns eitehr an empty Pod struct and error, or a populated
//PodConfig for the podname given
func getPod(podname string) (*parser.PodConfig, error) {
	var pod parser.PodConfig

	if config.UseSentinelConfig {
		sentinel, err := parser.ParseSentinelConfig(config.SentinelConfigFile)
		if err != nil {
			log.Print(err)
			return &pod, err
		}
		pod, err = sentinel.GetPod(podname)
		if err != nil {
			log.Fatal(err)
		}
		return &pod, err
	}
	if config.UseRedSkull {
		log.Print("Using RedSkull connection")
	}
	return &pod, nil
}

func Reset(pod *parser.PodConfig) error {
	// loop over list of sentinels, issue a reset
	sentinels, err := pod.GetSentinels()
	if err != nil {
		log.Print(err.Error())
	}
	resets := 0
	for _, s := range sentinels {
		sc, err := client.DialAddress(s)
		if err != nil {
			log.Print(err.Error())
			continue
		}
		err = sc.SentinelReset(pod.Name)
		if err != nil {
			log.Print(err.Error())
			continue
		}
		resets++
	}
	if resets != len(sentinels) {
		return fmt.Errorf("Only %d of %d sentinels were successfully reset", resets, len(sentinels))
	}
	return nil
}

// Failover() will issue a failover to at least one sentinel in the known
// sentinels list, returning an error none succeed
func Failover(pod *parser.PodConfig) error {
	// loop over list of sentinels, issue a failover
	// on first success return nil
	// fall through to returning an error
	success := true
	sentinels, err := pod.GetSentinels()
	if err != nil {
		return err
	}
	for _, s := range sentinels {
		sc, err := client.DialAddress(s)
		if err != nil {
			log.Print(err.Error())
			continue
		}
		success, err := sc.SentinelFailover(pod.Name)
		if err != nil {
			log.Print(err.Error())
			continue
		}
		if success {
			return nil
		}
	}
	if !success {
		return fmt.Errorf("No sentinels accepted the failover request")
	}
	return nil
}

// LiveSlaves() returns a list of connections to slaves. it can be empty if no
// slaves exist or no slaves are reachable
func LiveSlaves(pod parser.PodConfig) []*client.Redis {
	slaves := pod.KnownSlaves
	var live []*client.Redis
	for _, s := range slaves {
		sc, err := client.DialAddress(s)
		if err != nil {
			log.Print(err.Error())
			continue
		}
		live = append(live, sc)
	}
	return live
}

// ValidateSentinels() iterates over KnownSentinels, connecting to each This is
// useufl for confirming the number of known sentinels matches the number of
// sentinels available
func ValidateSentinels(pod *parser.PodConfig) (bool, error) {
	sentinels, err := pod.GetSentinels()
	if err != nil {
		return false, err
	}
	failed := 0
	connected := 0
	for _, s := range sentinels {
		sc, err := client.DialAddress(s)
		if err != nil {
			log.Print(s, err.Error())
			failed++
			continue
		}
		master, err := sc.SentinelMaster(pod.Name)
		if err != nil {
			log.Printf("[%s] %s", s, err.Error())
			failed++
			continue
		}
		if master.Name != pod.Name {
			log.Printf("Wierd, request master for pod '%s', got master for pod '%s'", pod.Name, master.Name)
			failed++
			continue
		} else {
			connected++
		}
	}
	if len(sentinels) > connected {
		return false, fmt.Errorf("%d of %d sentinels were contacted and has this pod in their list", connected, len(sentinels))
	}
	return true, nil
}
