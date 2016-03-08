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
	app.Version = "0.9.1"
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
