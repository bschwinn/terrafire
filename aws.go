package terrafire

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"log"

	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/route53"
)

var ICON_TERMINATED = "\xE2\x98\xA0"        // skull n bones
var ICON_RUNNING = "\xF0\x9F\x8D\xBB"       // cheers w/ beers
var ICON_SHUTTING_DOWN = "\xF0\x9F\x92\xA3" // bombora
var ICON_DEFAULT = "\xF0\x9F\x92\xA9"       // smiling poo (what else)

// CreateAWSSession - create a re-usable AWS session
func CreateAWSSession() *session.Session {
	sess, err := session.NewSession()
	if err != nil {
		panic(err)
	}
	return sess
}

// CreateEC2Service - create a re-usable AWS EC2 service
func CreateEC2Service(region string, sesh *session.Session) *ec2.EC2 {
	return ec2.New(sesh, &aws.Config{Region: aws.String(region)})
}

// CreateRoute53Service - create a re-usable AWS EC2 service
func CreateRoute53Service(sesh *session.Session) *route53.Route53 {
	return route53.New(sesh)
}

// GetGroupInstances - get a group's instances via call to describe instances
func GetGroupInstances(group GroupConfig, svc *ec2.EC2) ([]ec2.Instance, error) {

	filter := CreateGroupInstanceFilter(group)
	resp, err := svc.DescribeInstances(filter)
	if err != nil {
		return nil, err
	}

	instances := make([]ec2.Instance, 0)
	for idx := range resp.Reservations {
		for _, inst := range resp.Reservations[idx].Instances {
			instances = append(instances, *inst)
		}
	}
	return instances, nil
}

// util - create a run instance input based on our config
func createRunInstanceInput(inst EC2Instance) *ec2.RunInstancesInput {
	netSpec := &ec2.InstanceNetworkInterfaceSpecification{
		AssociatePublicIpAddress: aws.Bool(inst.AssociatePublicIP),
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
func CreateGroupInstanceFilter(group GroupConfig) *ec2.DescribeInstancesInput {
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

// CreateIDInstanceFilter - build a filter to get instances by id
func CreateIDInstanceFilter(idMap map[string]EC2Instance) *ec2.DescribeInstancesInput {
	var ids []string
	for id := range idMap {
		ids = append(ids, id)
	}
	flt := &ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice(ids),
	}
	return flt
}

// GetInstanceTag - get an instance tag value given a tag name
func GetInstanceTag(tagname string, instance ec2.Instance) string {
	for _, tag := range instance.Tags {
		if tagname == *tag.Key {
			return *tag.Value
		}
	}
	return ""
}

// GetInstanceStateIcon - get an instance tag value given a tag name
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

// RunInstances - run all the instances in the whole group
func RunInstances(svc *ec2.EC2, config RunConfig, instanceData map[string]EC2InstanceLive, logger *log.Logger) (map[string]EC2Instance, error) {
	instanceMap := make(map[string]EC2Instance, 0)
	for idx := range config.Tier.Instances {
		// create the instance input and launch
		inst := config.Tier.Instances[idx]
		inst.UserData = createInstanceUserData(config, inst, instanceData)
		ipt := createRunInstanceInput(inst)
		logger.Printf("Launching: %v\n", inst.Name)
		res, err := svc.RunInstances(ipt)
		if err != nil {
			return nil, err
		}

		// keep the new details in the instance map
		newInstanceID := res.Instances[0].InstanceId
		instanceMap[*newInstanceID] = inst

		// tag the newly launched instances
		_, errtag := svc.CreateTags(&ec2.CreateTagsInput{
			Resources: []*string{newInstanceID},
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
			panic(fmt.Errorf("ERROR Could not create tags for instance: %s, error: %s", *newInstanceID, errtag))
		}
	}
	return instanceMap, nil
}

// RunInstancesNoop - simulate a run
func RunInstancesNoop(config RunConfig, instanceData map[string]EC2InstanceLive, logger *log.Logger) map[string]EC2Instance {
	instanceMap := make(map[string]EC2Instance, 0)
	for idx := range config.Tier.Instances {
		inst := config.Tier.Instances[idx]
		inst.UserData = createInstanceUserData(config, inst, instanceData)
		logger.Printf("Launching (noop): %v\n", inst.Name)
		newInstanceID := fmt.Sprintf("instance_%s_%d", config.Tier.Name, idx)
		instanceMap[newInstanceID] = inst
	}
	return instanceMap
}

// GetInstances - get instance data
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

// GetInstancesNoop - generate fake instance data
func GetInstancesNoop(config RunConfig, instanceMap map[string]EC2Instance) map[string]*ec2.Instance {
	instanceData := make(map[string]*ec2.Instance, 0)
	for idx := range instanceMap {
		instConf := instanceMap[idx]
		inst := createFakeEC2Instance(instConf)
		instanceData[idx] = inst
	}
	return instanceData
}

// AssociateElasticIP - associate instances with any elastic IP addresses
func AssociateElasticIP(svc *ec2.EC2, runConf RunConfig, instanceData map[string]EC2InstanceLive, logger *log.Logger) error {
	for idx := range runConf.Tier.Instances {
		inst := runConf.Tier.Instances[idx]
		linst := instanceData[inst.Name]
		// associate any elastic IPs with the newly launched instance
		if !inst.AssociatePublicIP && (inst.ElasticIPID != "") {
			_, errip := svc.AssociateAddress(&ec2.AssociateAddressInput{
				AllocationId: aws.String(inst.ElasticIPID),
				InstanceId:   aws.String(linst.InstanceID),
			})
			if errip != nil {
				return errip
			}
		}
	}
	return nil
}

// UpdateRoute53 - updates the route53 "A" records for nodes in this tier
func UpdateRoute53(svc *route53.Route53, runConf RunConfig, instanceData map[string]EC2InstanceLive, logger *log.Logger) error {
	for idx := range runConf.Tier.Instances {
		inst := runConf.Tier.Instances[idx]
		if inst.Route53.ZoneID != "" && inst.Route53.Suffix != "" {
			linst := instanceData[inst.Name]
			fqdn := inst.Name + "." + inst.Route53.Suffix
			val := linst.PublicIpAddress
			if inst.Route53.RecordType == "CNAME" {
				val = linst.PublicDnsName
			}
			r53params := createRoute53Params("UPSERT", inst.Route53.RecordType, inst.Route53.ZoneID, fqdn, val, inst.Route53.TTL)
			resp, err := svc.ChangeResourceRecordSets(r53params)
			if err != nil {
				return err
			}
			logger.Println(resp)
		}
	}
	return nil
}

func createRoute53Params(action, recordType, zoneID, name, ipaddr string, ttl int64) *route53.ChangeResourceRecordSetsInput {
	params := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String(action),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(name),
						Type: aws.String(recordType),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String(ipaddr),
							},
						},
						TTL: aws.Int64(ttl),
					},
				},
			},
		},
		HostedZoneId: aws.String(zoneID),
	}
	return params
}

func createFakeEC2Instance(instConf EC2Instance) *ec2.Instance {
	inst := &ec2.Instance{
		PrivateDnsName:   aws.String(fmt.Sprintf("%s_PrivateDNS(computed)", instConf.Name)),
		PrivateIpAddress: aws.String(fmt.Sprintf("%s_PrivateIP(computed)", instConf.Name)),
	}
	if instConf.AssociatePublicIP {
		inst.PublicIpAddress = aws.String(fmt.Sprintf("%s_PublicIP(computed)", instConf.Name))
		inst.PublicDnsName = aws.String(fmt.Sprintf("%s_PublicDNS(computed)", instConf.Name))
	}
	return inst
}

// PostProcessInstances - runs the post-process script for each instance
func PostProcessInstances(groupConf GroupConfig, logger *log.Logger) error {
	count := groupConf.InstanceCount()
	var waiter sync.WaitGroup
	waiter.Add(count)
	for i := range groupConf.Tiers {
		tier := groupConf.Tiers[i]
		for j := range tier.Instances {
			inst := tier.Instances[j]
			logger.Printf("Running post launch on instance: %s, script: %+v", inst.Name, inst.PostLaunch)
			go func() {
				cmd := exec.Command(inst.PostLaunch.Command, inst.PostLaunch.Args...)
				cmd.Dir = inst.PostLaunch.Dir
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				err := cmd.Run()
				if err != nil {
					logger.Printf("Error running post launch script: %s", err)
				}
				waiter.Done()
			}()
		}
	}
	waiter.Wait()
	return nil
}

// PostProcessInstancesNoop - runs the post-process script for each instance
func PostProcessInstancesNoop(groupConf GroupConfig, logger *log.Logger) error {
	count := groupConf.InstanceCount()
	logger.Printf("running post launch for %d nodes", count)
	for i := range groupConf.Tiers {
		tier := groupConf.Tiers[i]
		for j := range tier.Instances {
			inst := tier.Instances[j]
			logger.Printf("Running (noop) post launch on instance: %s, script: %s", inst.Name, inst.PostLaunch)
		}
	}
	return nil
}
