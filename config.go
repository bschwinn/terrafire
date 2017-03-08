package terrafire

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/ec2"
)

// BaseConfig - Our main config struct definition
type BaseConfig struct {
	Debug        bool          `mapstructure:"debug"`
	ShowTags     bool          `mapstructure:"showtags"`
	TemplatePath string        `mapstructure:"templatepath"`
	Group        string        `mapstructure:"group"`
	Groups       []GroupConfig `mapstructure:"groups"`
}

// TerraFireRunConfig - config composite of base config, current group and current tier
type RunConfig struct {
	BaseConfig
	Group GroupConfig
	Tier  EC2InstanceTier
}

func (bc BaseConfig) String() string {
	s := fmt.Sprintf("TerraFireConfig{ debug: %t, show-tags: %t, group: %s, template path: %s, groups [", bc.Debug, bc.ShowTags, bc.Group, bc.TemplatePath)
	for _, gc := range bc.Groups {
		s = s + gc.String()
	}
	s = s + "]}"
	return s
}

// GroupConfig  - group config
type GroupConfig struct {
	Name         string            `mapstructure:"name"`
	Region       string            `mapstructure:"region"`
	PuppetMaster string            `mapstructure:"puppetmaster"`
	YumRepo      string            `mapstructure:"yumrepo"`
	Tiers        []EC2InstanceTier `mapstructure:"tiers"`
}

func (gc GroupConfig) String() string {
	s := fmt.Sprintf("GroupConfig {Name : %s, Tiers: [", gc.Name)
	for _, t := range gc.Tiers {
		s = s + t.String()
	}
	s = s + "]}"
	return s
}

// EC2InstanceTier  - Teir config for a group
type EC2InstanceTier struct {
	Name      string        `mapstructure:"name"`
	Instances []EC2Instance `mapstructure:"instances"`
}

func (et EC2InstanceTier) String() string {
	s := fmt.Sprintf("EC2InstanceTier{ Name: %s, Instances: [", et.Name)
	for _, t := range et.Instances {
		s = s + fmt.Sprintf("EC2Instance{ %v }", t)
	}
	s = s + "]}"
	return s
}

// GetInstance - get an instance in this tier
func (et EC2InstanceTier) GetInstance(name string) *EC2Instance {
	for _, t := range et.Instances {
		if name == t.Name {
			return &t
		}
	}
	return nil
}

// EC2Instance - main config struct for an instance
type EC2Instance struct {
	Type              string            `mapstructure:"type"`
	Name              string            `mapstructure:"name"`
	AMI               string            `mapstructure:"ami"`
	Zone              string            `mapstructure:"zone"`
	Subnet            string            `mapstructure:"subnet"`
	SecGroups         string            `mapstructure:"secgroups"`
	KeyName           string            `mapstructure:"keyname"`
	Hostname          string            `mapstructure:"hostname"`
	ElasticIPID       string            `mapstructure:"elasticipid"`
	Route53           Route53Config     `mapstructure:"route53"`
	AssociatePublicIP bool              `mapstructure:"assocpublic"`
	Bootstrap         BootTemplates     `mapstructure:"bootstrap"`
	UserData          string            `mapstructure:"userdata"`
	Properties        map[string]string `mapstructure:"properties"`
}

func (inst EC2Instance) String() string {
	return fmt.Sprintf("name: %s, hostname: %s, zone: %s, type: %s, subnet: %s, sec-groups: %s, ami: %s, public ip? %t, elastic ip: %s, route53 zone: %s, user data: %s", inst.Name, inst.Hostname, inst.Zone, inst.Type, inst.Subnet, inst.SecGroups, inst.AMI, inst.AssociatePublicIP, inst.ElasticIPID, inst.Route53.ZoneID, inst.UserData)
}

// Route53Config - struct for Route53 upsert/delete
type Route53Config struct {
	ZoneID string `mapstructure:"zoneid"`
	Suffix string `mapstructure:"suffix"`
	TTL    int64  `mapstructure:"ttl"`
}

// BootTemplates - struct for header/body/footer templates for UserData
type BootTemplates struct {
	Header  string `mapstructure:"header"`
	Content string `mapstructure:"content"`
	Footer  string `mapstructure:"footer"`
}

// EC2InstanceLive - config plus some live instance properties
type EC2InstanceLive struct {
	EC2Instance
	InstanceID       string
	PrivateDnsName   string
	PrivateIpAddress string
	PublicIpAddress  string
	PublicDnsName    string
}

// Apply - pull in live instance properties
func (inst EC2InstanceLive) Apply(liveInst *ec2.Instance) {
	inst.PrivateDnsName = *liveInst.PrivateDnsName
	inst.PrivateIpAddress = *liveInst.PrivateIpAddress
	inst.PublicDnsName = *liveInst.PublicDnsName
	inst.PublicIpAddress = *liveInst.PublicIpAddress
}

func (inst EC2InstanceLive) String() string {
	s := inst.EC2Instance.String()
	s = s + fmt.Sprintf("private ip: %s, private dns: %s", inst.PrivateIpAddress, inst.PrivateDnsName)
	return s
}
