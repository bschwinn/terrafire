package terrafire

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"log"
)

var ICON_TERMINATED = "\xE2\x98\xA0"        // skull n bones
var ICON_RUNNING = "\xF0\x9F\x8D\xBB"       // cheers w/ beers
var ICON_SHUTTING_DOWN = "\xF0\x9F\x92\xA3" // bombora
var ICON_DEFAULT = "\xF0\x9F\x92\xA9"       // smiling poo (what else)

// create a re-usable AWS session
func createAWSSession() *session.Session {
	sess, err := session.NewSession()
	if err != nil {
		panic(err)
	}
	return sess
}

// create a re-usable AWS EC2 service
func CreateEC2Service(region string) *ec2.EC2 {
	sesh := createAWSSession()
	return ec2.New(sesh, &aws.Config{Region: aws.String(region)})
}

// util - get a group's instances via call to describe instances
func GetGroupInstances(group GroupConfig, svc *ec2.EC2) []ec2.Instance {

	filter := createGroupInstanceFilter(group)
	resp, err := svc.DescribeInstances(filter)
	if err != nil {
		panic(err)
	}

	instances := make([]ec2.Instance, 0)
	for idx := range resp.Reservations {
		for _, inst := range resp.Reservations[idx].Instances {
			instances = append(instances, *inst)
		}
	}
	return instances
}

// util - create a run instance input based on our config
func createRunInstanceInput(inst EC2Instance) *ec2.RunInstancesInput {
	netSpec := &ec2.InstanceNetworkInterfaceSpecification{
		AssociatePublicIpAddress: aws.Bool(true),
		DeviceIndex:              aws.Int64(0),
		SubnetId:                 aws.String(inst.Subnet),
		Groups:                   aws.StringSlice(strings.Split(inst.SecGroups, ",")),
	}
	return &ec2.RunInstancesInput{
		ImageId:           aws.String(inst.AMI),
		InstanceType:      aws.String(inst.Type),
		KeyName:           aws.String(inst.KeyName),
		MaxCount:          aws.Int64(1),
		MinCount:          aws.Int64(1),
		UserData:          aws.String(inst.UserData),
		NetworkInterfaces: []*ec2.InstanceNetworkInterfaceSpecification{netSpec},
	}
}

// util - build a filter to check tags "Launcher=Terrafire" and "TerrafireGroup=group"
func createGroupInstanceFilter(group GroupConfig) *ec2.DescribeInstancesInput {
	flt := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:Launcher"),
				Values: []*string{aws.String("Terrafire")},
			},
			{
				Name:   aws.String("tag:TerrafireGroup"),
				Values: []*string{aws.String(group.Name)},
			},
		},
	}
	return flt
}

// util - build a filter to get instances by id
func CreateIdInstanceFilter(idMap map[string]string) *ec2.DescribeInstancesInput {
	var ids []string
	for id := range idMap {
		ids = append(ids, id)
	}
	flt := &ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice(ids),
	}
	return flt
}

// util - get an instance tag value given a tag name
func GetInstanceTag(tagname string, instance ec2.Instance) string {
	for _, tag := range instance.Tags {
		if tagname == *tag.Key {
			return *tag.Value
		}
	}
	return ""
}

// util - get an instance tag value given a tag name
func GetInstanceStateIcon(state string) string {
	res := ICON_DEFAULT
	switch state {
	case ec2.InstanceStateNameTerminated:
		res = ICON_TERMINATED
	case ec2.InstanceStateNameRunning:
		res = ICON_RUNNING
	case ec2.InstanceStateNameShuttingDown:
		res = ICON_SHUTTING_DOWN
	}
	return res
}

// util - run a list of instances returned a list of instance ids
func RunInstances(svc *ec2.EC2, config TerraFireRunConfig, instanceData map[string]EC2InstanceLive, logger *log.Logger) map[string]string {
	instanceMap := make(map[string]string, 0)
	for idx := range config.Tier.Instances {
		inst := config.Tier.Instances[idx]
		inst.UserData = createInstanceUserData(config, inst, instanceData)
		ipt := createRunInstanceInput(inst)
		logger.Printf("Launching: %v\n", inst.Name)
		res, err := svc.RunInstances(ipt)
		if err != nil {
			panic(err)
		}
		newInstanceId := res.Instances[0].InstanceId
		instanceMap[*newInstanceId] = inst.Name
		_, errtag := svc.CreateTags(&ec2.CreateTagsInput{
			Resources: []*string{newInstanceId},
			Tags: []*ec2.Tag{
				{
					Key:   aws.String("Name"),
					Value: aws.String(inst.Name),
				},
				{
					Key:   aws.String("Launcher"),
					Value: aws.String("Terrafire"),
				},
				{
					Key:   aws.String("TerrafireGroup"),
					Value: aws.String(config.Group.Name),
				},
			},
		})
		if errtag != nil {
			panic(fmt.Errorf("ERROR Could not create tags for instance: %s, error: %s", *newInstanceId, errtag))
		}
	}
	return instanceMap
}

func RunInstancesNoop(config TerraFireRunConfig, instanceData map[string]EC2InstanceLive, logger *log.Logger) map[string]string {
	instanceMap := make(map[string]string, 0)
	for idx := range config.Tier.Instances {
		inst := config.Tier.Instances[idx]
		inst.UserData = createInstanceUserData(config, inst, instanceData)
		logger.Printf("Launching (noop): %v\n", inst.Name)
		newInstanceId := fmt.Sprintf("instance_%s_%d", config.Tier.Name, idx)
		instanceMap[newInstanceId] = inst.Name
	}
	return instanceMap
}

// util - run a list of instances returned a list of instance ids
func GetInstances(svc *ec2.EC2, flt *ec2.DescribeInstancesInput) map[string]*ec2.Instance {
	instanceData := make(map[string]*ec2.Instance, 0)
	launched, err := svc.DescribeInstances(flt)
	if err != nil {
		panic(err)
	}
	for resIdx := range launched.Reservations {
		res := launched.Reservations[resIdx]
		for instIdx := range res.Instances {
			inst := res.Instances[instIdx]
			instanceData[*inst.InstanceId] = inst
		}
	}
	return instanceData
}

func GetInstancesNoop(instanceMap map[string]string) map[string]*ec2.Instance {
	instanceData := make(map[string]*ec2.Instance, 0)
	for idx := range instanceMap {
		inst := createFakeEC2Instance(idx)
		instanceData[idx] = inst
	}
	return instanceData
}

func createFakeEC2Instance(id string) *ec2.Instance {
	inst := &ec2.Instance{
		PrivateDnsName:   aws.String(fmt.Sprintf("%s_PrivateDNS(computed)", id)),
		PrivateIpAddress: aws.String(fmt.Sprintf("%s_PrivateIP(computed)", id)),
		PublicIpAddress:  aws.String(fmt.Sprintf("%s_PublicIP(computed)", id)),
		PublicDnsName:    aws.String(fmt.Sprintf("%s_PublicDNS(computed)", id)),
	}
	return inst
}
