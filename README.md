# Terrafire

Terrafire is an incredibly simplistic provisioning system.  It is inspired by Terraform in it's general approach.


## What is Terrafire?  How does it differ from Terraform?

- Terrafire expresses infrastructure in config files like Terraform, but processes the information differently
- Terrafire's main goal is to, as quick as possible, bootstrap an AWS instance enough to run puppet (or provisioning system du jour)
- Terrafire does not support infrastructure morphism - it will never "update" an instance
- Terrafire is designed to complain and do nothing in the event of ambiguities
- Terrafire does not keep state, it relies on AWS to keep state - if your config get's out of sync with reality, it will complain and fail.
- Terrafire has no concept of provisioners, it handles all bootstrapping of an instance via user data file templates
- Terrafire has a first class notion of groups which can really be thought of as environments
- Terrafire groups also have the notion of tiers which can be thought of as layers in an environment
- Terrafire does not support true dependencies, however, each new tier is launched with the launch results of the previous tier
- Terrafire treats the AWS tag "name" specially, for example, you can't launch two nodes in the same group with tag:Name = web-server
- Terrafire also uses AWS tags to mark what it creates


## Terrafire Commands

- groups - this command lists all configured groups
- live(group) - this command will show all live infrastructure with the group's tags
- plan(group) - this command will show the plan to create the groups infrastructure.  It will warn if it encounters any existing instances with the same name.
- apply(group) - this command will execute the plan to create the groups infrastructure.  It will fail if it encounters any existing instances with the same name.
- destroy(group) - this command will destroy the group's infrastructure.  It will fail for two reasons; 1) if it can not find existing instances with 
the same name and 2) if it encounters live infrastructure without a corresponding configuration entry.


## How do I use this thing?

1. Clone the repo and build the executable:
```
cd cmd/terrafire
go build
```
2. Create your own terrafire working directory.  This can be a clean directory where you can start a repo or an existing repo.
3. Create your own config file using ./cmd/terrafire/config/config.yml as an example.  The file should be named "config.yml" and should live in a "config" directory which is relative to the executable.
4. If using User Data templates, ensure you configure the templates directory appropriately.
5. Invoke terrafire to list all your groups:
```
./terrafire groups
```
6. Invoke terrafire in plan mode to see what it will do, if you need more info, just add a "-d" flag for debugging:
```
./terrafire -g your-group-name plan
```


## FAQ

*Why Terrafire?*

I built this for me, not (necessarily) for you.


*Why not Terraform?*

Tried it.  Fought far too long and hard to be able to set an alternate (dynamic) host name for each instance via a UserData file (for puppet).
I tried provisioners but that slowed things down immensely and since I already had this working with user data files...
Oh, and the state stuff just feels gross.  Oy yeah, throw in the principled objections to adding if/for...


*Is Terrafire better than Terraform?*

No.


*Will it ever be?*

No.


*Seriously, why do you build this?*

I wanted the config semantics (or similar) of Terraform without practically ANY of it's processing - philosophical differences n all.