// +build !redskull
package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/sentinel-tools/sconf-parser"
	"github.com/therealbill/libredis/client"
)

var sentinel parser.SentinelConfig

//getPod(podname) returns eitehr an empty Pod struct and error, or a populated
//PodConfig for the podname given
func getPod(podname string) (*parser.PodConfig, error) {
	var pod parser.PodConfig
	var err error
	sentinel, err = parser.ParseSentinelConfig(config.SentinelConfigFile)
	if err != nil {
		log.Print(err)
		return &pod, err
	}
	pod, err = sentinel.GetPod(podname)
	if err != nil {
		log.Fatal(err)
	}
	return &pod, err
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

// CheckAuth() will attempt to connect to the master and validate we can auth
// by issuing a ping
func CheckAuth(pod *parser.PodConfig) (map[string]bool, error) {
	addr := fmt.Sprintf("%s:%s", pod.MasterIP, pod.MasterPort)
	results := make(map[string]bool)
	invalid := false
	dc := client.DialConfig{Address: addr, Password: pod.Authpass}
	c, err := client.DialWithConfig(&dc)
	if err != nil {
		if !strings.Contains(err.Error(), "invalid password") {
			log.Print("Unable to connect to %s. Error: %s", addr, err.Error())
		}
		results["master"] = false
	} else {
		err = c.Ping()
		if err != nil {
			log.Print(err)
			results["master"] = false
			invalid = true
		} else {
			results["master"] = true
		}
	}

	for _, slave := range LiveSlaves(*pod) {
		sid := fmt.Sprintf(slave.Address())
		if slave.Ping() != nil {
			results[sid] = false
			invalid = true
			continue
		} else {
			results[sid] = true
		}
	}
	if invalid {
		err = errors.New("At least one node in pod failed auth check")
	}
	return results, err
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

// Remove() will attempt to connect to every KnownSentinel and issue a
// "sentinel remove <podname>", However, since if a KnownSentinel is offline,
// it will not be told to remove it, this command may not fully clean up a pod.
// In this scenario you will need to log into the failed sentinel when it comes
// back up and execute the command there to clean it up.
func Remove(pod *parser.PodConfig) (bool, error) {
	sentinels, err := pod.GetSentinels()
	if err != nil {
		return false, err
	}
	failed := 0
	for _, s := range sentinels {
		sc, err := client.DialAddress(s)
		if err != nil {
			log.Print(s, err.Error())
			failed++
			continue
		}
		ok, err := sc.SentinelRemove(pod.Name)
		if err != nil {
			log.Print(s, err.Error())
			failed++
			continue
		}
		if !ok {
			log.Print("Sentinel replied with unknown status. Manual verification recommended.")
			failed++
			continue
		}
	}
	if failed != 0 {
		return false, fmt.Errorf("Not all sentinels had successful replies. Manual intervention required.")
	}
	return true, nil
}

// TreeWalk() will attempt to walk the pod list for a given pod's IPs.  The
// idea here is to hopefully find all pods which are, due to misconfiguration
// in sentinel, sharing one or more IPs with this pod.
func TreeWalk(pod *parser.PodConfig) []parser.PodConfig {
	fmt.Printf("\nWalking Pod %s\n", pod.Name)
	iso := true
	var connecteds []parser.PodConfig
	for pname, rpod := range sentinel.ManagedPodConfigs {
		if pname == pod.Name {
			// skip self
			continue
		}
		master_address := fmt.Sprintf("%s:%s", pod.MasterIP, pod.MasterPort)
		if pod.MasterIP == rpod.MasterIP {
			fmt.Printf("[%s] Master IP also listed as master for %s\n", pod.Name, pname)
			connecteds = append(connecteds, rpod)
			iso = false
		}
		for _, s := range rpod.KnownSlaves {
			if master_address == s {
				fmt.Printf("[%s] Master IP is ALSO listed as slave of %s\n", pod.Name, pname)
				connecteds = append(connecteds, rpod)
				iso = false
			}
		}
		for _, ms := range pod.KnownSlaves {
			if strings.Contains(ms, rpod.MasterIP) {
				fmt.Printf("[%s] Slave IP(%s) also listed as master for %s\n", pod.Name, ms, pname)
				connecteds = append(connecteds, rpod)
				iso = false
			}
			for _, s := range rpod.KnownSlaves {
				if ms == s {
					fmt.Printf("[%s] Slave IP(%s) is ALSO listed as slave of %s\n", pod.Name, ms, pname)
					connecteds = append(connecteds, rpod)
					iso = false
				}
			}
		}
	}
	if iso {
		fmt.Printf("[%s] Is properly isolated\n", pod.Name)
	} else {
		res, err := CheckAuth(pod)
		if err != nil {
			log.Print(err)
			for k, v := range res {
				log.Printf("[%s] %s: %t", podname, k, v)
			}
		} else {
			log.Print("Auth valid")
		}
		fmt.Printf("These pods are intermingled with %s:\n", pod.Name)
		for _, mpod := range connecteds {
			fmt.Printf("\t%s\n", mpod.Name)
		}
	}
	return connecteds
}
