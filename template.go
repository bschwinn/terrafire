package terrafire

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"path"
	"text/template"
)

type EC2UserDataTemplateContext struct {
	EC2Instance
	Environment  string
	PuppetMaster string
	Launched     map[string]EC2InstanceLive
}

const TEMPLATE_GLOB_PATTERN string = "*.tmpl"

// util - run the template(s) to create the user data to pass to the instance (the bootstrap script)
func createInstanceUserData(config TerraFireRunConfig, inst EC2Instance, instanceData map[string]EC2InstanceLive) string {
	// setup template context and functions
	ctx := EC2UserDataTemplateContext{inst, config.Group.Name, config.Group.PuppetMaster, instanceData}
	// TODO - define more template funcs based on "EC2InstanceLive"
	funcMap := template.FuncMap{
		"PrivateIP": func(s string) string {
			if inst, ok := instanceData[s]; ok {
				ip := inst.PrivateIpAddress
				return ip
			}
			return ""
		},
	}
	glob := path.Join(config.TemplatePath, TEMPLATE_GLOB_PATTERN)
	templates, terr := template.New("terrafire").Funcs(funcMap).ParseGlob(glob)
	if terr != nil {
		panic(terr)
	}

	// render all the templates
	res := ""
	if inst.Bootstrap.Header != "" {
		res = res + runTemplate(inst.Bootstrap.Header, templates, ctx)
	}
	if inst.Bootstrap.Content != "" {
		res = res + runTemplate(inst.Bootstrap.Content, templates, ctx)
	}
	if inst.Bootstrap.Footer != "" {
		res = res + runTemplate(inst.Bootstrap.Footer, templates, ctx)
	}
	// TODO figure out how to debug this properly
	//fmt.Println(res)

	encoded := base64.StdEncoding.EncodeToString([]byte(res))
	return encoded
}

func runTemplate(name string, templates *template.Template, ctx EC2UserDataTemplateContext) string {
	var buffy bytes.Buffer
	w := bufio.NewWriter(&buffy)
	terr := templates.ExecuteTemplate(w, name, ctx)
	if terr != nil {
		panic(terr)
	}
	w.Flush()
	return buffy.String()
}
