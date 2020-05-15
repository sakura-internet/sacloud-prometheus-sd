package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
	"github.com/sacloud/libsacloud/v2/sacloud"
	"github.com/sacloud/libsacloud/v2/sacloud/search"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	sacloudLabel           = model.MetaLabelPrefix + "sacloud_"
	sacloudLabelZone       = sacloudLabel + "zone"
	sacloudLabelResourceID = sacloudLabel + "resource_id"
	sacloudLabelName       = sacloudLabel + "name"
	sacloudLabelIP         = sacloudLabel + "ip"
	sacloudLabelTag        = sacloudLabel + "instances_tags"
)

type Config struct {
	Token        string   `yaml:"sacloud_token"`
	Secret       string   `yaml:"sacloud_token_secret"`
	Zone         string   `yaml:"sacloud_zone"`
	BaseTags     []string `yaml:"base_tags"`
	Targets      []Target `yaml:"targets"`
	HostNameType string   `yaml:"host_name_type"` // modify_disk or server_name
}
type Target struct {
	Service        string   `yaml:"service"`
	Tags           []string `yaml:"tags"`
	IgnoreTags     []string `yaml:"ignore_tags"`
	Ports          []int    `yaml:"ports"`
	InterfaceIndex int      `yaml:"interface_index"` // start from 0
}

func main() {
	var durationSec = flag.Int("i", 0, "refresh interval. if not specified once run.")
	var configPath = flag.String("config", "config.yml", "config file path")
	var generatedPath = flag.String("generated", "./generated.yml", "generated file path")
	var token = flag.String("token", "", "sakura cloud API token")
	var secret = flag.String("secret", "", "sakura cloud API secret")
	var zone = flag.String("zone", "", "sakura cloud zone name")

	flag.Parse()
	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	// token
	if config.Token == "" {
		config.Token = *token
	}
	if config.Token == "" {
		config.Token = os.Getenv("SAKURACLOUD_ACCESS_TOKEN")
	}
	// secret
	if config.Secret == "" {
		config.Secret = *secret
	}
	if config.Secret == "" {
		config.Secret = os.Getenv("SAKURACLOUD_ACCESS_TOKEN_SECRET")
	}
	// zone
	if config.Zone == "" {
		config.Zone = *zone
	}
	if config.Zone == "" {
		config.Zone = os.Getenv("SAKURACLOUD_ZONE")
	}

	for {
		err := generate(config, *generatedPath)
		if err != nil {
			log.Printf("[ERROR] generate error %v", err)
		}
		if *durationSec == 0 {
			if err != nil {
				os.Exit(1)
			}
			break
		}
		time.Sleep(time.Duration(*durationSec) * time.Second)
	}
}

func generate(config Config, generatedFilePath string) error {
	err := os.MkdirAll(filepath.Dir(generatedFilePath), 0777)
	if err != nil {
		return err
	}

	var list = make([]interface{}, 0)
	for _, target := range config.Targets {
		client := sacloud.NewClient(config.Token, config.Secret)

		var tags []string
		tags = append(tags, config.BaseTags...)
		tags = append(tags, target.Tags...)

		ctx := context.Background()
		rawServers, err := listServers(ctx, config.Zone, client, tags)
		if err != nil {
			return err
		}

		var servers []*sacloud.Server
		for _, server := range rawServers {
			isIgnored := false
			for _, ignore := range target.IgnoreTags {
				if server.HasTag(ignore) {
					isIgnored = true
					break
				}
			}
			if !isIgnored {
				servers = append(servers, server)
			}
		}

		targetGroups, err := buildMetadata(servers, target, config)
		if err != nil {
			return err
		}

		for _, targetGroup := range targetGroups {
			g, err := targetGroup.MarshalYAML()
			if err != nil {
				return err
			}
			list = append(list, g)
		}
	}
	file, err := os.Create(generatedFilePath)
	if err != nil {
		return err
	}
	yb, err := yaml.Marshal(list)
	_, err = fmt.Fprint(file, string(yb))
	if err != nil {
		return err
	}
	err = file.Close()
	if err != nil {
		return err
	}
	return nil
}

func loadConfig(path string) (Config, error) {
	var c Config

	yamlFile, err := ioutil.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		return Config{}, err
	}

	return c, nil
}

// listServers return a list of sacloud.Server
// if tag != nil, return only tagged server
func listServers(ctx context.Context, zone string, client *sacloud.Client, tags []string) ([]*sacloud.Server, error) {
	op := sacloud.NewServerOp(client)
	cnd := &sacloud.FindCondition{
		Filter: search.Filter{
			search.Key("Tags"): search.TagsAndEqual(tags...),
		},
	}
	res, err := op.Find(ctx, zone, cnd)
	if err != nil {
		return nil, err
	}

	return res.Servers, nil
}

func buildMetadata(servers []*sacloud.Server, target Target, config Config) ([]*targetgroup.Group, error) {
	var targetGroups []*targetgroup.Group

	for _, server := range servers {
		tg := &targetgroup.Group{
			Source: fmt.Sprintf("sacloud"),
		}

		if len(server.Interfaces) <= target.InterfaceIndex {
			continue
		}
		IPAddress := server.Interfaces[target.InterfaceIndex].GetUserIPAddress() // maybe private address (connect to switch)

		if IPAddress == "" {
			IPAddress = server.Interfaces[0].GetIPAddress() // maybe shared global ip
		}
		if IPAddress == "" {
			IPAddress = server.Interfaces[0].GetUserIPAddress() // ip configured by API
		}
		if IPAddress == "" {
			// not found ip addresses
			continue
		}

		targetLabels := model.LabelSet{}
		for _, port := range target.Ports {
			address := fmt.Sprintf("%s:%d", IPAddress, port)
			targetLabels[model.AddressLabel] = model.LabelValue(address)
		}

		tg.Targets = append(tg.Targets, targetLabels)

		labels := model.LabelSet{
			sacloudLabelZone:       model.LabelValue(server.Zone.Name),
			sacloudLabelResourceID: model.LabelValue(strconv.FormatInt(int64(server.ID), 10)),
			sacloudLabelName:       model.LabelValue(server.Name),
			sacloudLabelIP:         model.LabelValue(IPAddress),
		}

		if len(server.Tags) > 0 {
			sanitizedTags := make([]string, 0, len(server.Tags))
			for _, t := range server.Tags {
				st := strutil.SanitizeLabelName(t)
				sanitizedTags = append(sanitizedTags, st)
			}
			joinedTags := "," + strings.Join(sanitizedTags, ",") + ","
			labels[sacloudLabelTag] = model.LabelValue(joinedTags)
		}

		if config.HostNameType == "modify_disk" {
			labels["hostname"] = model.LabelValue(server.HostName)
		} else {
			labels["hostname"] = model.LabelValue(server.Name)
		}
		labels["service"] = model.LabelValue(target.Service)

		tg.Labels = labels

		targetGroups = append(targetGroups, tg)
	}

	return targetGroups, nil
}
