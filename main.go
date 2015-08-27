package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"text/template"

	"github.com/codegangsta/cli"
	"github.com/kelseyhightower/envconfig"
	"github.com/sentinel-tools/sconf-parser"
	"github.com/therealbill/libredis/client"
)

type LaunchConfig struct {
	SentinelConfigFile string
}

var (
	config  LaunchConfig
	enc     *json.Encoder
	pod     *parser.PodConfig
	app     *cli.App
	podname string
	cfile   string
)

func init() {
	err := envconfig.Process("podmanager", &config)
	if err != nil {
		log.Fatal(err)
	}
}

func checkError(err error) {
	if err != nil {
		log.Fatalf("Error: %s", err)
	}
}

func SetConfigFile(c *cli.Context) error {
	cfile = c.GlobalString("sentinelconfig")
	return nil
}

func main() {
	app = cli.NewApp()
	app.Name = "pod-manager"
	app.Usage = "Interact with a Sentinel using configuration data"
	app.Version = "0.5.1"
	app.EnableBashCompletion = true
	author := cli.Author{Name: "Bill Anderson", Email: "therealbill@me.com"}
	app.Authors = append(app.Authors, author)
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "sentinelconfig, s",
			Value: "/etc/redis/sentinel.conf",
			Usage: "Location of the sentinel config file",
		},
	}
	app.Before = SetConfigFile

	app.Commands = []cli.Command{
		{
			Name:  "pod",
			Usage: "Pod based actions",
			Subcommands: []cli.Command{
				{
					Name:   "failover",
					Usage:  "Failover the given pod",
					Action: FailoverPod,
					Before: beforePodCommand,
				},
				{
					Name:   "validatesentinels",
					Usage:  "Validate sentinels for the given pod",
					Action: ValidatePodSentinels,
					Before: beforePodCommand,
				},
				{
					Name:   "checkauth",
					Usage:  "Validate authentication for the given pod",
					Action: CheckPodAuth,
					Before: beforePodCommand,
				},
				{
					Name:   "walk",
					Usage:  "Walk the given pod",
					Action: WalkPod,
					Before: beforePodCommand,
				},
				{
					Name:   "reset",
					Usage:  "Reset given pod",
					Action: ResetPod,
					Before: beforePodCommand,
				},
				{
					Name:   "remove",
					Usage:  "Remove pod",
					Action: RemovePod,
					Before: beforePodCommand,
					Flags: []cli.Flag{
						cli.BoolFlag{
							Name:  "archive, a",
							Usage: "Archive pod configuration",
						},
					},
				},
				{
					Name:   "info",
					Usage:  "Show info for pod",
					Action: ShowInfo,
					Before: beforePodCommand,
					Flags: []cli.Flag{
						cli.BoolFlag{
							Name:  "json, j",
							Usage: "Output only JSON",
						},
					},
				},
				{
					Name:   "changepass",
					Usage:  "Change pod's password",
					Action: changePodAuthentication,
					Before: beforePodCommand,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "oldpass, o",
							Usage: "Existing password",
						},
						cli.StringFlag{
							Name:  "newpass, n",
							Usage: "New password",
						},
					},
				},
			},
		},
	}
	app.Run(os.Args)
}

func beforePodCommand(c *cli.Context) (err error) {
	//podname := c.String("name")
	args := c.Args()
	if len(args) == 0 {
		log.Fatal("Need a podname as first argument")
	}
	podname = args[0]
	pod, _ = getPod(podname)
	return nil
}

func FailoverPod(c *cli.Context) {
	err := Failover(pod)
	checkError(err)
	log.Print("Failover inititated")
}

func ValidatePodSentinels(c *cli.Context) {
	val, err := ValidateSentinels(pod)
	checkError(err)
	if val {
		log.Print("sentinels validated")
	} else {
		log.Print("constellation state invalid for the pod")
	}

}

func ResetPod(c *cli.Context) {
	err := Reset(pod)
	checkError(err)
	log.Print("Reset inititated")
}

func ShowInfo(c *cli.Context) {
	if c.Bool("json") {
		enc = json.NewEncoder(os.Stdout)
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

func RemovePod(c *cli.Context) {
	if c.Bool("archive") {
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

func WalkPod(c *cli.Context) {
	log.Printf("WalkPod called for pod %s", c.String("n"))
	walked := make(map[string]bool)
	x := TreeWalk(*pod)
	if len(x) > 0 {
		walked[pod.Name] = true
		for _, mp := range x {
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

func CheckPodAuth(c *cli.Context) {
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

func changePodAuthentication(c *cli.Context) {
	oldpass := c.String("oldpass")
	newpass := c.String("newpass")
	if pod.Authpass != oldpass {
		log.Fatal("Old password given does not match configured password.")
	}
	log.Printf("Updating authentication information for master of %s", pod.Name)
	conn := pod.Client()
	conn.ConfigSet("masterauth", newpass)
	conn.ConfigSet("requirepass", newpass)
	pod.Authpass = newpass
	slaves, err := pod.GetSlaves()
	// Note: this only triggers if there was an error obtaining the list. If
	// there was no error but there are no slaves, nothing will happen here.
	if err != nil {
		log.Printf("Unable to pull slaves for %s, reverting password change on master", pod.Name)
		conn := pod.Client()
		conn.ConfigSet("masterauth", oldpass)
		conn.ConfigSet("requirepass", oldpass)
		pod.Authpass = oldpass
		log.Fatal("Error from Sentinel:", err)
	}
	switch len(slaves) {
	case 1:
		log.Print("Updating slave")
	case 0:
		log.Print("No slaves to update")
	default:
		log.Printf("Updating %d slaves", len(slaves))
	}

	updated := 0
	for _, s := range slaves {
		log.Printf("Updating Slave %s", s)
		slave, err := client.DialWithConfig(&client.DialConfig{Address: s, Password: oldpass})
		if err != nil {
			log.Printf("Unable to connect to %s! You will need to manually adjust the password for this sentinel.", s)
			continue
		}
		slave.ConfigSet("masterauth", newpass)
		slave.ConfigSet("requirepass", newpass)
		updated++
	}
	if len(slaves) > 0 {
		log.Printf("%d of %d slaves updated", updated, len(slaves))
	}
	updated = 0
	sentinels, err := pod.GetSentinels()
	for _, s := range sentinels {
		//log.Printf("Updating Sentinel %s", s)
		sentinel, err := client.DialAddress(s)
		if err != nil {
			log.Printf("Unable to connect to %s! You will need to manually adjust the password for this sentinel.", s)
			continue
		}
		sentinel.SentinelSetString(pod.Name, "auth-pass", newpass)
		updated++
	}
	if updated != len(sentinels) {
		log.Printf("%d of %d sentinels updated", updated, len(sentinels))
		log.Printf("Not all Known Sentinels were updated! You will need to manually check and ensure the new password is set in the failed sentinels")
	} else {
		log.Printf("All %d Sentinels Updated.", updated)
		log.Printf("You should now be able to validate with 'redis-cli -h %s -p %s -a %s PING'", pod.MasterIP, pod.MasterPort, newpass)
	}
}
