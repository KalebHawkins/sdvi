/*
Copyright © 2022 Kaleb Hawkins <Kaleb_Hawkins@outlook.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/apenella/go-ansible/pkg/execute"
	"github.com/apenella/go-ansible/pkg/options"
	"github.com/apenella/go-ansible/pkg/playbook"
	"github.com/apenella/go-ansible/pkg/stdoutcallback/results"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// configureCmd represents the configure command
var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "configure a generic app server",
	Long: `configure a generic app server.
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := generateAnsibleVars(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to generate ansible vars: %s\n", err)
			os.Exit(1)
		}

		if err := generateAnsibleInv(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to generate ansible inventory: %s\n", err)
			os.Exit(1)
		}

		if err := generatePlaybook(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to generate ansible playbook: %s\n", err)
			os.Exit(1)
		}

		RunPlaybook()
	},
}

func init() {
	rootCmd.AddCommand(configureCmd)
}

type AnsibleVars struct {
	RhelUsername          interface{} `yaml:"rhel_username"`
	RhelPassword          interface{} `yaml:"rhel_password"`
	RhelPoolIDs           []string    `yaml:"rhel_poolids"`
	DNSSuffixSearchList   []string    `yaml:"dnsSuffixSearchList"`
	NTPServers            []string    `yaml:"ntpServers"`
	DNSServers            []string    `yaml:"dnsServers"`
	CrowdstrikeTag        string      `yaml:"crowdstrikeTag"`
	CrowdstrikeCustomerID string      `yaml:"crowdstrikeCustomerID"`
	QualysCustomerID      string      `yaml:"qualysCustomerID"`
	QualysActivationID    string      `yaml:"qualysActivationID"`
	SplunkUsername        string      `yaml:"splunkUsername"`
	SplunkPassword        string      `yaml:"splunkPassword"`
	SplunkDeployServer    string      `yaml:"splunkDeployServer"`
	RealmControllers      []string    `yaml:"realmControllers"`
	RealmUsername         string      `yaml:"realmUsername"`
	RealmPassword         string      `yaml:"realmPassword"`
	RealmGroup            string      `yaml:"realmGroup"`
	RealmOU               string      `yaml:"realmOU"`
}

func generateAnsibleVars() error {
	fmt.Println("Generating Ansible variables...")
	ansVars := AnsibleVars{
		RhelUsername:          viper.GetString("redhat.username"),
		RhelPassword:          viper.GetString("redhat.password"),
		RhelPoolIDs:           viper.GetStringSlice("redhat.pools"),
		DNSSuffixSearchList:   viper.GetStringSlice("dns.suffix"),
		DNSServers:            viper.GetStringSlice("dns.servers"),
		NTPServers:            viper.GetStringSlice("ntpServers"),
		CrowdstrikeTag:        viper.GetString("crowdstrike.tag"),
		CrowdstrikeCustomerID: viper.GetString("crowdstrike.customerID"),
		QualysCustomerID:      viper.GetString("qualys.customerID"),
		QualysActivationID:    viper.GetString("qualys.activationID"),
		SplunkUsername:        viper.GetString("splunk.deployUsername"),
		SplunkPassword:        viper.GetString("splunk.deployPassword"),
		SplunkDeployServer:    viper.GetString("splunk.deployServer"),
		RealmControllers:      viper.GetStringSlice("realm.controllers"),
		RealmUsername:         viper.GetString("realm.username"),
		RealmPassword:         viper.GetString("realm.password"),
		RealmGroup:            viper.GetString("realm.group"),
		RealmOU:               viper.GetString("realm.organizationUnit"),
	}

	yml, err := yaml.Marshal(ansVars)
	if err != nil {
		return fmt.Errorf("failed to marshal ansible configuration")
	}

	err = ioutil.WriteFile("./ansible/vars.yml", yml, 0755)
	if err != nil {
		return fmt.Errorf("failed to write file ./ansible/vars.yml: %v", err)
	}

	fmt.Println("Wrote ansible variables to ./ansible/vars.yml")
	return nil
}

func generateAnsibleInv() error {
	fmt.Println("Generating ansible inventory...")
	var ansInv = `---
all:
  children:
    generated:
      hosts:`

	var servers []*Server
	err := viper.UnmarshalKey("servers", &servers)
	if err != nil {
		return fmt.Errorf("failed to unmarshal servers")
	}

	for _, srv := range servers {
		ansInv += fmt.Sprintf("\n%*s:", 8+len(srv.Name), srv.Name)
	}

	err = ioutil.WriteFile("./ansible/inv.yml", []byte(ansInv), 0755)
	if err != nil {
		return fmt.Errorf("failed to write file ./ansible/inv.yml: %v", err)
	}

	fmt.Println("Wrote ansible inventory to ./ansible/inv.yml")
	return nil
}

func generatePlaybook() error {
	fmt.Println("Generating playbook...")
	var plybk = `---
- hosts: generated
  gather_facts: true
  vars_files:
    - vars.yml

  roles:
    - common
    - disclaimer
    - crowdstrike
    - qualys
    - splunkforwarder
    - domainjoin
`

	httpProxy := viper.GetString("ansible.httpProxy")
	httpsProxy := viper.GetString("ansible.httpsProxy")
	if httpProxy != "" || httpsProxy != "" {
		plybk = fmt.Sprintf("%s\n  environment:",
			plybk)
	}

	if httpProxy != "" {
		plybk = fmt.Sprintf("%s\n    http_proxy: %s", plybk, httpProxy)
	}
	if httpsProxy != "" {
		plybk = fmt.Sprintf("%s\n    https_proxy: %s", plybk, httpsProxy)
	}

	err := ioutil.WriteFile("./ansible/site.yml", []byte(plybk), 0755)
	if err != nil {
		return fmt.Errorf("failed to write file ./ansible/site.yml: %v", err)
	}

	fmt.Println("Wrote ansible playbook to ./ansible/site.yml")
	return nil
}

func RunPlaybook() {

	ansibleSSHKey := viper.GetString("ansible.sshKeyPath")
	ansibleUsername := viper.GetString("ansible.username")

	apco := &options.AnsibleConnectionOptions{
		PrivateKey: ansibleSSHKey,
		User:       ansibleUsername,
	}

	apo := &playbook.AnsiblePlaybookOptions{
		Inventory: "ansible/inv.yml",
	}

	// apeo := &options.AnsiblePrivilegeEscalationOptions{
	// 	Become:        true,
	// 	BecomeMethod:  "sudo",
	// 	BecomeUser:    "root",
	// 	AskBecomePass: true,
	// }

	plybk := &playbook.AnsiblePlaybookCmd{
		Playbooks:         []string{"ansible/site.yml"},
		Options:           apo,
		ConnectionOptions: apco,
		Exec: execute.NewDefaultExecute(
			execute.WithEnvVar("ANSIBLE_FORCE_COLOR", "true"),
			execute.WithTransformers(
				results.Prepend("Ansible Playbook Running"),
			),
		),
	}

	err := plybk.Run(context.Background())
	if err != nil {
		panic(err)
	}
}
