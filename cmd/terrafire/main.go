package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/bschwinn/terrafire"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var debug bool
var selectedGroup string
var infoLog *log.Logger
var debugLog *log.Logger
var errorLog *log.Logger

const destroyOk = "YES"

func init() {
	flag.BoolVarP(&debug, "debug", "d", false, "debugging flag, will dump viper/cobra data")
	flag.StringVarP(&selectedGroup, "group", "g", "", "Group name, required fall all commands except default (groups).")
}

// config defaults and merged global instance, structs in config.go
var ourConfig terrafire.BaseConfig

// main routine - kick off one of the sub-commands
func main() {

	// parse all flags
	flag.Parse()

	// parse configuration
	viper.SetConfigName("config") // name of config file (without extension)
	viper.AddConfigPath("./config")
	viper.SetConfigType("yml")
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("fatal error config file: %s", err))
	}
	viper.BindPFlags(flag.CommandLine)
	err = viper.Unmarshal(&ourConfig)
	if err != nil {
		fmt.Printf("fatal error unmarshalling config file: %s", err)
		os.Exit(1)
	}

	ourConfig.Group = selectedGroup

	// setup loggers
	debugger := ioutil.Discard
	if ourConfig.Debug {
		debugger = os.Stdout
	}
	initLoggers(os.Stdout, debugger, os.Stderr)

	// debugging
	if ourConfig.Debug {
		debugConfig()
	}

	// execution
	if err := RootCmd.Execute(); err != nil {
		errorLog.Fatal(err)
	}

	os.Exit(0)
}

// sub-command - show all the defined groups
func runGroups(cmd *cobra.Command, args []string) error {
	infoLog.Println("All Groups:")
	for _, grp := range ourConfig.Groups {
		infoLog.Println(" - ", grp.Name)
	}
	return nil
}

// sub-command - show all the defined hosts in a group
func runHosts(cmd *cobra.Command, args []string) error {
	group, err := getGroup()
	if err != nil {
		errorLog.Fatal(err)
	}

	for i := range group.Tiers {
		tier := group.Tiers[i]
		for j := range tier.Instances {
			inst := tier.Instances[j]
			infoLog.Printf("%s", inst.Hostname)
		}
	}

	return nil
}

// sub-command - show instance info for live instances in the group
func runInfo(cmd *cobra.Command, args []string) error {
	group, err := getGroup()
	if err != nil {
		errorLog.Fatal(err)
	}

	// get existing instances in group
	infoLog.Println("Live resourcces in group:")
	sesh := terrafire.CreateAWSSession()
	ec2 := terrafire.CreateEC2Service(group.Region, sesh)

	instances, err := terrafire.GetGroupInstances(group, ec2)
	if err != nil {
		errorLog.Fatal(err)
	}
	for i := range instances {
		ec2Inst := instances[i]
		infoLog.Printf(" - %s (ec2 instance) - id: %s, state: %s", terrafire.GetInstanceTag("Name", ec2Inst), aws.StringValue(ec2Inst.InstanceId), terrafire.GetInstanceStateIcon(aws.StringValue(ec2Inst.State.Name)))
	}

	return nil
}

// sub-command - show the plan for a group
func runPlan(cmd *cobra.Command, args []string) error {
	group, err := getGroup()
	if err != nil {
		errorLog.Fatal(err)
	}

	// create the plan
	sesh := terrafire.CreateAWSSession()
	svc := terrafire.CreateEC2Service(group.Region, sesh)
	plan, planerr := createPlan(group, svc)
	if planerr != nil {
		errorLog.Fatal(planerr)
	}

	// show any errors else show the plan (what would be done)
	if len(plan.Errors) > 0 {
		infoLog.Println("Error(s) in plan")
		for errIdx := range plan.Errors {
			infoLog.Println(plan.Errors[errIdx])
		}
	} else {

		infoLog.Println("Plan looks OK, running....")

		allInstanceData := make(map[string]terrafire.EC2InstanceLive, 0)
		for i := range plan.Group.Tiers {
			tier := plan.Group.Tiers[i]
			trc := terrafire.RunConfig{BaseConfig: ourConfig, Group: group, Tier: tier}
			instanceMap := terrafire.RunInstancesNoop(trc, allInstanceData, infoLog)

			// record instance details for reference in subsequent tiers
			instanceMapLive := terrafire.GetInstancesNoop(trc, instanceMap)

			err := combineInstanceData(tier, instanceMap, instanceMapLive, allInstanceData)
			if err != nil {
				panic(err)
			}

			if ourConfig.Debug {
				debugLog.Printf("Tier created: %v\n", instanceMapLive)
				debugLog.Printf("All Instance Data: %v\n", allInstanceData)
			}
		}
		posterr := terrafire.PostProcessInstancesNoop(group, infoLog)
		if posterr != nil {
			errorLog.Fatal(posterr)
		}

	}
	return nil
}

// sub-command - show the plan for a group
func runApply(cmd *cobra.Command, args []string) error {
	group, err := getGroup()
	if err != nil {
		errorLog.Fatal(err)
	}

	// create the plan
	sesh := terrafire.CreateAWSSession()
	svc := terrafire.CreateEC2Service(group.Region, sesh)
	r53 := terrafire.CreateRoute53Service(sesh)
	plan, planerr := createPlan(group, svc)
	if planerr != nil {
		errorLog.Fatal(planerr)
	}

	// show any errors else create the earth
	if len(plan.Errors) > 0 {
		infoLog.Println("Error(s) in plan")
		for errIdx := range plan.Errors {
			infoLog.Println(plan.Errors[errIdx])
		}
	} else {

		infoLog.Println("Plan looks OK, running....")

		allInstanceData := make(map[string]terrafire.EC2InstanceLive, 0)
		for i := range plan.Group.Tiers {
			// run the instances in this tier
			tier := plan.Group.Tiers[i]
			trc := terrafire.RunConfig{BaseConfig: ourConfig, Group: group, Tier: tier}
			instanceMap, err := terrafire.RunInstances(svc, trc, allInstanceData, infoLog)
			if err != nil {
				errorLog.Fatal(err)
			}

			// wait for instances to launch
			infoLog.Println(" - Waiting for instances to launch:", instanceMap)
			flt := terrafire.CreateIDInstanceFilter(instanceMap)
			err2 := svc.WaitUntilInstanceRunning(flt)
			if err2 != nil {
				errorLog.Fatal(err2)
			}
			infoLog.Println(" - Instances have launched, looking up instance info....")

			// record instance details for reference in subsequent tiers
			instanceMapLive := terrafire.GetInstances(svc, flt)

			cerr := combineInstanceData(tier, instanceMap, instanceMapLive, allInstanceData)
			if cerr != nil {
				errorLog.Fatal(cerr)
			}

			elasticerr := terrafire.AssociateElasticIP(svc, trc, allInstanceData, infoLog)
			if elasticerr != nil {
				errorLog.Fatal(elasticerr)
			}

			r53err := terrafire.UpdateRoute53(r53, trc, allInstanceData, infoLog)
			if r53err != nil {
				errorLog.Fatal(r53err)
			}

			if ourConfig.Debug {
				debugLog.Printf(" - Tier created: %v\n", instanceMapLive)
				debugLog.Printf(" - All Instance Data: %v\n", allInstanceData)
			}
		}

		// wait for instances to come up and then run the post launch scripts
		svc.WaitUntilInstanceRunning(terrafire.CreateGroupInstanceFilter(group))
		posterr := terrafire.PostProcessInstances(group, infoLog)
		if posterr != nil {
			errorLog.Fatal(posterr)
		}
	}
	return nil
}

// sub-command - run any post launch commands for a group
func runPost(cmd *cobra.Command, args []string) error {
	group, err := getGroup()
	if err != nil {
		errorLog.Fatal(err)
	}

	posterr := terrafire.PostProcessInstances(group, infoLog)
	if posterr != nil {
		errorLog.Fatal(posterr)
	}

	return nil
}

// sub-command - destroy a group
func runDestroy(cmd *cobra.Command, args []string) error {
	group, err := getGroup()
	if err != nil {
		errorLog.Fatal(err)
	}

	// create the plan
	sesh := terrafire.CreateAWSSession()
	svc := terrafire.CreateEC2Service(group.Region, sesh)
	plan, planerr := createDestroyPlan(group, svc)
	if planerr != nil {
		errorLog.Fatal(planerr)
	}

	// show any errors else prompt before total annihilation
	if len(plan.Errors) > 0 {
		infoLog.Println("Error(s) in destroy plan")
		for errIdx := range plan.Errors {
			infoLog.Println(plan.Errors[errIdx])
		}
	} else {
		infoLog.Println("Plan looks OK, Are you sure you want to destroy these resources?")

		for i := range plan.Group.Tiers {
			tier := plan.Group.Tiers[i]
			for idx := range tier.Instances {
				inst := tier.Instances[idx]
				infoLog.Printf(" - %s (%s)\n", inst.Name, "ec2 instance")
			}
		}

		// Prompt and read for "yes" in order to destroy all the things
		reader := bufio.NewReader(os.Stdin)
		infoLog.Printf("If you're absolutely sure you want to destroy the \nabove resources, enter \"%s\" to proceed.", destroyOk)
		text, _ := reader.ReadString('\n')
		debugLog.Printf("Instance IDs that are about to be destroyed: %v", plan.InstanceIds)
		if strings.TrimSpace(text) == destroyOk {
			flt := &ec2.TerminateInstancesInput{
				InstanceIds: aws.StringSlice(plan.InstanceIds),
			}
			termOut, err := svc.TerminateInstances(flt)
			if err != nil {
				panic(err)
			}
			debugLog.Printf("Terminate output: %v", termOut)
		} else {
			infoLog.Print("No problem, we won't be destroying anything this time. \nFeel free to re-run destroy when you're feeling more destructive.")
		}

	}
	return nil
}

// util - intialize the loggers
func initLoggers(infoWriter, debugWriter, errorWriter io.Writer) {
	if ourConfig.Debug {
		infoLog = log.New(infoWriter, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)

	} else {
		infoLog = log.New(infoWriter, "", 0)

	}
	debugLog = log.New(debugWriter, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)
	errorLog = log.New(errorWriter, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
}

// util - get a single group configuration by name, error if not found
func getGroup() (terrafire.GroupConfig, error) {
	if ourConfig.Group == "" {
		return terrafire.GroupConfig{}, fmt.Errorf("terrafire group can not be empty")
	}
	for _, grp := range ourConfig.Groups {
		if grp.Name == ourConfig.Group {
			if grp.Region != "" {
				return grp, nil
			}
			return terrafire.GroupConfig{}, fmt.Errorf("terraform group '%s' must have a region defined", ourConfig.Group)
		}
	}
	return terrafire.GroupConfig{}, fmt.Errorf("terrafire group '%s' not found, run the 'groups' command to see all groups", ourConfig.Group)
}

// map instanceMapLive (map of id to live instance data) and instanceMap (map of id to name) into allInstanceData (map of name to live instance data)
func combineInstanceData(tier terrafire.EC2InstanceTier, instanceMap map[string]terrafire.EC2Instance, instanceMapLive map[string]*ec2.Instance, allInstanceData map[string]terrafire.EC2InstanceLive) error {
	if len(instanceMap) != len(instanceMapLive) {
		return fmt.Errorf("all instances have NOT launched, attempted: %d, launched: %d", len(instanceMap), len(instanceMapLive))
	}
	for k, v := range instanceMap {
		// create new instance based on config and apply live instance data
		liveInst := instanceMapLive[k]
		newInst := terrafire.EC2InstanceLive{EC2Instance: *tier.GetInstance(v.Name)}
		newInst.InstanceID = aws.StringValue(liveInst.InstanceId)
		newInst.PrivateIpAddress = aws.StringValue(liveInst.PrivateIpAddress)
		newInst.PrivateDnsName = aws.StringValue(liveInst.PrivateDnsName)
		newInst.PublicIpAddress = aws.StringValue(liveInst.PublicIpAddress)
		newInst.PublicDnsName = aws.StringValue(liveInst.PublicDnsName)
		allInstanceData[v.Name] = newInst
	}
	return nil
}

// util - debug the current configuration
func debugConfig() {
	vprDbg := ""
	delim := ""
	vKeys := viper.AllKeys()
	for k := range vKeys {
		key := vKeys[k]
		vprDbg = vprDbg + fmt.Sprintf("%s %s = %v", delim, key, viper.Get(key))
		delim = ","

	}
	debugLog.Printf("Terrafire(viper) { %s }", vprDbg)
	debugLog.Printf("Terrafire(parsed): { %v }", ourConfig)
}

/*************  PLAN STUFF *************/

// TerrafirePlan - all the things that will be created as well as a list of any errors
type TerrafirePlan struct {
	Group  terrafire.GroupConfig
	Errors []string
}

// TerrafireDestroyPlan - all the things that will be created as well as a list of any errors
type TerrafireDestroyPlan struct {
	TerrafirePlan
	InstanceIds []string
}

// create the plan of attack for instantiating all the things
func createPlan(group terrafire.GroupConfig, svc *ec2.EC2) (TerrafirePlan, error) {

	plan := TerrafirePlan{Group: group}
	instances, used, err := gatherPlanData(group, svc)
	if err != nil {
		return plan, err
	}

	// step 2 - check for existing live instances and build Error list
	errors := make([]string, 0)
	for instIdx := range instances {
		inst := instances[instIdx]
		// it's ok if there's one that's terminated
		if aws.StringValue(inst.State.Name) == ec2.InstanceStateNameTerminated {
			continue
		}
		tagName := terrafire.GetInstanceTag("Name", inst)
		if tagName == "" {
			// TODO figure out what this case is ?
			errorLog.Println("Null tagname found for instance. ", inst.InstanceId)
			continue
		}
		if exists, _ := used[tagName]; exists {
			errors = append(errors, "Instance already exists: "+tagName)
		}
	}
	plan.Errors = errors

	return plan, nil
}

// create the plan of attack for DESTROYING all the things
func createDestroyPlan(group terrafire.GroupConfig, svc *ec2.EC2) (TerrafireDestroyPlan, error) {

	plan := TerrafirePlan{Group: group}
	instances, configed, err := gatherPlanData(group, svc)
	if err != nil {
		return TerrafireDestroyPlan{plan, []string{}}, err
	}

	// step 2 - validate plan
	errors := make([]string, 0)
	existing := make(map[string]bool, len(instances))
	existingIds := make([]string, 0)
	for instIdx := range instances {
		inst := instances[instIdx]
		tagName := terrafire.GetInstanceTag("Name", inst)
		if tagName == "" {
			errorLog.Println("Null tagname found for instance. ", inst.InstanceId)
			continue
		}
		// check that our configuration matches actual AWS instances
		existing[tagName] = true
		existingIds = append(existingIds, aws.StringValue(inst.InstanceId))
		if exists, _ := configed[tagName]; !exists {
			errors = append(errors, "Instance: \""+tagName+"\" exists but is not configured!!")
		}
	}
	// check that our configuration doesn't try to destroy non-existing nodes
	for k := range configed {
		if ok, _ := existing[k]; !ok {
			errors = append(errors, "Instance: \""+k+"\" does not exist!!")
		}
	}
	plan.Errors = errors
	destPlan := TerrafireDestroyPlan{TerrafirePlan: plan, InstanceIds: existingIds}
	return destPlan, nil
}

// gather up the config, the live instance info and make a list of configured (taken) names
func gatherPlanData(group terrafire.GroupConfig, svc *ec2.EC2) ([]ec2.Instance, map[string]bool, error) {

	// get existing instances in group
	instances, err := terrafire.GetGroupInstances(group, svc)
	if err != nil {
		return nil, nil, err
	}

	// check for empty tiers (illegal)
	for _, tier := range group.Tiers {
		if len(tier.Instances) < 1 {
			return nil, nil, errors.New("a tier must contain at least one instance")
		}
	}

	// create a map of existing instance names
	names := make(map[string]bool)
	for i := range group.Tiers {
		tier := group.Tiers[i]
		for idx := range tier.Instances {
			names[tier.Instances[idx].Name] = true
		}
	}
	return instances, names, nil
}
