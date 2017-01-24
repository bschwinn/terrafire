package terrafire

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// Our main config struct definition
type TerraFireConfig struct {
	Debug        bool          `name:"debug"`
	ShowTags     bool          `name:"showtags"`
	TemplatePath string        `name:"templatepath"`
	Group        string        `name:"group"`
	Groups       []GroupConfig `name:"groups"`
}

type TerraFireRunConfig struct {
	TerraFireConfig
	Group GroupConfig
	Tier  EC2InstanceTier
}

func (tfc TerraFireConfig) String() string {
	s := fmt.Sprintf("TerraFireConfig{ debug: %t, show-tags: %t, group: %s, template path: %s, groups [", tfc.Debug, tfc.ShowTags, tfc.Group, tfc.TemplatePath)
	for _, gc := range tfc.Groups {
		s = s + gc.String()
	}
	s = s + "]}"
	return s
}

type GroupConfig struct {
	Name         string            `name:"name"`
	Region       string            `name:"region"`
	PuppetMaster string            `name:"puppetmaster"`
	Tiers        []EC2InstanceTier `name:"tiers"`
	TemplateDir  string            `name:"template-dir"`
}

func (gc GroupConfig) String() string {
	s := fmt.Sprintf("GroupConfig {Name : %s, Tiers: [", gc.Name)
	for _, t := range gc.Tiers {
		s = s + t.String()
	}
	s = s + "]}"
	return s
}

type EC2InstanceTier struct {
	Name      string        `name:"name"`
	Instances []EC2Instance `name:"instances"`
}

func (et EC2InstanceTier) String() string {
	s := fmt.Sprintf("EC2InstanceTier{ Name: %s, Instances: [", et.Name)
	for _, t := range et.Instances {
		s = s + fmt.Sprintf("EC2Instance{ %s }", t.Name)
	}
	s = s + "]}"
	return s
}

func (et EC2InstanceTier) GetInstance(name string) *EC2Instance {
	for _, t := range et.Instances {
		if name == t.Name {
			return &t
		}
	}
	return nil
}

type EC2Instance struct {
	Type       string            `name:"type"`
	Name       string            `name:"name"`
	AMI        string            `name:"ami"`
	Zone       string            `name:"zone"`
	Subnet     string            `name:"subnet"`
	SecGroups  string            `name:"secgroups"`
	KeyName    string            `name:"keyname"`
	Hostname   string            `name:"hostname"`
	Bootstrap  TerraFireBoot     `name:"bootstrap"`
	UserData   string            `name:"userdata"`
	Properties map[string]string `name:"properties"`
}

func (inst EC2Instance) String() string {
	return fmt.Sprintf("name: %s, hostname: %s, zone: %s, type: %s, subnet: %s, sec-groups: %s, ami: %s, user data: %s", inst.Name, inst.Hostname, inst.Zone, inst.Type, inst.Subnet, inst.SecGroups, inst.AMI, inst.UserData)
}

type TerraFireBoot struct {
	Header  string `name:"header"`
	Content string `name:"content"`
	Footer  string `name:"footer"`
}

type EC2InstanceLive struct {
	EC2Instance
	PrivateDnsName   string
	PrivateIpAddress string
	PublicIpAddress  string
	PublicDnsName    string
}

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
