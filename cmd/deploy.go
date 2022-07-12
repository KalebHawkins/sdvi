/*
Copyright © 2022 Kaleb Hawkins <KalebHawkins@outlook.com>

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
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/vmware/govmomi/govc/cli"
	_ "github.com/vmware/govmomi/govc/device"
	_ "github.com/vmware/govmomi/govc/vm"
	_ "github.com/vmware/govmomi/govc/vm/disk"
)

type VCenter struct {
	Url          string `yaml:"url"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	Template     string `yaml:"template"`
	Datastore    string `yaml:"datastore"`
	Network      string `yaml:"network"`
	ResourcePool string `yaml:"resourcepool"`
}

type Server struct {
	Name      string `yaml:"name"`
	Cpu       int    `yaml:"cpu"`
	MemoryMB  int    `yaml:"memoryMB"`
	IPAddress string `yaml:"ipaddress"`
	Netmask   string `yaml:"netmask"`
	Gateway   string `yaml:"gateway"`
	AppDiskGB int    `yaml:"appDisk"`
}

func init() {
	rootCmd.AddCommand(deployCmd)
}

func setupEnvironment() error {
	var vc VCenter
	if err := viper.UnmarshalKey("vcenter", &vc); err != nil {
		return err
	}

	envMap := map[string]string{
		"GOVC_URL":           vc.Url,
		"GOVC_USERNAME":      vc.Username,
		"GOVC_PASSWORD":      vc.Password,
		"GOVC_TEMPLATE":      vc.Template,
		"GOVC_DATASTORE":     vc.Datastore,
		"GOVC_NETWORK":       vc.Network,
		"GOVC_RESOURCE_POOL": vc.ResourcePool,
		"GOVC_INSECURE":      "true",
	}

	for k, v := range envMap {
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}

	return nil
}

func runGOVC(args ...string) int {
	return cli.Run(args)
}

func cloneVM(s *Server) error {
	vmCfg := []string{
		"vm.clone",
		"-vm", os.Getenv("GOVC_TEMPLATE"),
		"-on=false",
		fmt.Sprintf("-c=%d", s.Cpu),
		fmt.Sprintf("-m=%d", s.MemoryMB),
		fmt.Sprintf("-net=%s", os.Getenv("GOVC_NETWORK")),
		"-net.adapter=vmxnet3",
		s.Name,
	}

	if rtn := runGOVC(vmCfg...); rtn != 0 {
		return fmt.Errorf("failed to create virtual machine %s", s.Name)
	}

	return nil
}

func createVMDisk(s *Server) error {
	if s.AppDiskGB == 0 {
		return fmt.Errorf("appDisk not specified in configuration file")
	}

	diskCfg := []string{
		"vm.disk.create",
		"-vm", s.Name,
		"-name", fmt.Sprintf("%s/%s_001", s.Name, s.Name),
		"-size", fmt.Sprintf("%dG", s.AppDiskGB),
		"-thick=true",
	}

	if rtn := runGOVC(diskCfg...); rtn != 0 {
		return fmt.Errorf("failed to create disk for virtual machine %s", s.Name)
	}

	return nil
}

func setNicStartConnected(s *Server) error {
	connectCfg := []string{
		"device.connect",
		"-vm", s.Name,
		"ethernet-0",
	}
	if rtn := runGOVC(connectCfg...); rtn != 0 {
		return fmt.Errorf("failed to set vmxnet3 adapter to start connected on virtual machine %s", s.Name)
	}

	return nil
}

func setIPAddress(s *Server) error {
	netCfg := []string{
		"vm.customize",
		"-vm", s.Name,
		"-ip", s.IPAddress,
		"-netmask", s.Netmask,
		"-gateway", s.Gateway,
	}

	if rtn := runGOVC(netCfg...); rtn != 0 {
		return fmt.Errorf("failed to set ip address of %s", s.Name)
	}

	return nil
}

func powerOn(s *Server) error {
	pwrCmd := []string{
		"vm.power", "-on", s.Name,
	}

	if rtn := runGOVC(pwrCmd...); rtn != 0 {
		return fmt.Errorf("failed to power on %s", s.Name)
	}

	return nil
}

func deployPackage(s *Server) error {
	if err := cloneVM(s); err != nil {
		return err
	}
	if err := createVMDisk(s); err != nil {
		return err
	}
	if err := setNicStartConnected(s); err != nil {
		return err
	}
	if err := setIPAddress(s); err != nil {
		return err
	}
	if err := powerOn(s); err != nil {
		return err
	}

	return nil
}

// deployCmd represents the deploy command
var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "deploy virtual infrastructure",
	Long:  `deploy virtual infrastructure`,
	Run: func(cmd *cobra.Command, args []string) {

		fmt.Println("Setting up environment variables...")
		if err := setupEnvironment(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to setup enviornment variables: %s\n", err)
			os.Exit(1)
		}

		fmt.Println("Unmarshaling server structures...")
		var srvs []*Server
		if err := viper.UnmarshalKey("servers", &srvs); err != nil {
			fmt.Fprintf(os.Stderr, "failed to unmarshal servers: %s\n", err)
			os.Exit(1)
		}

		for _, srv := range srvs {
			if err := deployPackage(srv); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			if err := deployPackage(srv); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
			if err := deployPackage(srv); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}
	},
}
