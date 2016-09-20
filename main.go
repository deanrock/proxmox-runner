package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"strings"
	"time"

	"os"

	"github.com/deanrock/go-proxmox"
	dhcp "github.com/krolaw/dhcp4"
	"golang.org/x/crypto/ssh"
)

func DHCPServer(networkInterface string, handler *DHCPHandler) {
	dhcp.ListenAndServeIf(networkInterface, handler)
}

func PublicKeyFile(file string) ssh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil
	}
	return ssh.PublicKeys(key)
}

func main() {
	handler := &DHCPHandler{
		ip:            net.IP{192, 168, 150, 1},
		leaseDuration: 2 * time.Minute,
		start:         net.IP{192, 168, 150, 2},
		leaseRange:    30,
		leases:        LeaseMap{m: make(map[int]lease, 10)},
		options: dhcp.Options{
			dhcp.OptionSubnetMask:       []byte{255, 255, 255, 0},
			dhcp.OptionRouter:           []byte(net.IP{192, 168, 150, 1}), // Presuming Server is also your router
			dhcp.OptionDomainNameServer: []byte{8, 8, 8, 8},
		},
	}

	go DHCPServer("vmbr2", handler)

	p, err := proxmox.NewProxMox("onehundredandten", "root", os.Getenv("PASSWORD"))
	data, err := p.Get("nodes")
	fmt.Println(data)
	fmt.Println(err)
	if err != nil {
		fmt.Println(err)
		return
	}

	nodes, err := p.Nodes()
	if err != nil {
		fmt.Println(err)
		return
	}

	var templateQemu proxmox.QemuVM
	var targetNode proxmox.Node

	for _, node := range nodes {
		qemu, err := node.Qemu()
		if err != nil {
			return
		}

		for _, value := range qemu {
			if value.Template == 1 && value.Name == "osx-clean-ssh" {
				fmt.Println("PROXMOX => found template:", value.Name)
				templateQemu = value
				targetNode = node

				break
			}
		}
	}

	i, err := p.NextVMId()
	if err != nil {
		return
	}
	UPid, err := templateQemu.Clone(i, "bluhec", targetNode.Node)

	qemu, err := targetNode.Qemu()
	if err != nil {
		fmt.Println(err)
		return
	}

	tasks, err := p.Tasks()
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, task := range tasks {
		if task.UPid == UPid {
			fmt.Println("PROXMOX => waiting for clone task to finish")
			status, err := task.WaitForStatus("stopped", 60)
			fmt.Println("UPid error ", err, "; status", status)
		}
	}

	for _, value := range qemu {
		if value.VMId == i {
			fmt.Println("PROXMOX => VM found")

			err = value.Start()
			fmt.Println("PROXMOX => VM starting", err)

			err = value.WaitForStatus("running", 60)
			fmt.Println("PROXMOX => VM started", err)

			config, err := value.Config()
			if err != nil {

			}

			mac := ""

			for _, net := range config.Net {
				for _, v := range net {
					if strings.Count(v, ":") == 5 {
						mac = strings.ToLower(v)
					}
					continue
				}
			}

			found := ""
			if mac != "" {
				fmt.Println("PROXMOX => VM MAC:", mac)

				for {
					ip, err := handler.IPAddressForMAC(mac)
					if ip != "" && err == nil {
						fmt.Println("DHCP => Found IP:", ip)
						found = ip
						break
					}
					time.Sleep(1 * time.Second)
				}
			}

			fmt.Println("SSH => connecting to", found)
			count := 1

			for {
				sshConfig := &ssh.ClientConfig{
					User: "user",
					Auth: []ssh.AuthMethod{
						PublicKeyFile("/home/dean/.ssh/id_rsa"),
					},
				}

				client, err := ssh.Dial("tcp", found+":22", sshConfig)
				if err != nil {
					fmt.Println("SSH => try", count, ": can't run remote command: "+err.Error())
				} else {

					session, err := client.NewSession()
					if err != nil {
						client.Close()
						//return nil, nil, err
						fmt.Println("SSH => try", count, ": can't run remote command: "+err.Error())
					} else {
						fmt.Println("SSH => Executing uptime")
						out, err := session.CombinedOutput("uptime")
						if err != nil {
							fmt.Println("SSH => try", count, ": can't run remote command: "+err.Error())
						} else {
							fmt.Println(string(out))
							client.Close()
							break
						}
					}
				}

				time.Sleep(2 * time.Second)

				count++
				if count > 9 {
					break
				}
			}

			fmt.Println("PROXMOX => Stop and remove machine")
			value.Stop()
			value.Delete()
		}
	}
}
